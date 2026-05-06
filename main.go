package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/fullstorydev/grpcurl"
	"github.com/jhump/protoreflect/grpcreflect"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	reflectpb "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
)

type GRPCCallInput struct {
	Target        string            `json:"target" jsonschema:"host:port, for example localhost:50051"`
	Method        string            `json:"method" jsonschema:"full method, for example helloworld.Greeter/SayHello"`
	RequestJSON   json.RawMessage   `json:"request_json" jsonschema:"JSON request body"`
	Plaintext     bool              `json:"plaintext" jsonschema:"use plaintext HTTP/2 instead of TLS"`
	UseReflection bool              `json:"use_reflection" jsonschema:"use gRPC server reflection"`
	ProtoFiles    []string          `json:"proto_files,omitempty" jsonschema:"paths to .proto files"`
	ImportPaths   []string          `json:"import_paths,omitempty" jsonschema:"proto import paths"`
	Headers       map[string]string `json:"headers,omitempty" jsonschema:"request metadata headers"`
	TimeoutMS     int               `json:"timeout_ms,omitempty" jsonschema:"timeout in milliseconds"`
}

type GRPCCallOutput struct {
	ResponseJSON string `json:"response_json"`
}

func grpcCall(ctx context.Context, _ *mcp.CallToolRequest, in GRPCCallInput) (*mcp.CallToolResult, GRPCCallOutput, error) {
	if in.Target == "" {
		return nil, GRPCCallOutput{}, fmt.Errorf("target is required")
	}
	if in.Method == "" {
		return nil, GRPCCallOutput{}, fmt.Errorf("method is required")
	}
	if len(in.RequestJSON) == 0 {
		in.RequestJSON = json.RawMessage(`{}`)
	}

	timeout := 10 * time.Second
	if in.TimeoutMS > 0 {
		timeout = time.Duration(in.TimeoutMS) * time.Millisecond
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var dialOpts []grpc.DialOption
	if in.Plaintext {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(nil, "")))
	}

	cc, err := grpc.NewClient(in.Target, dialOpts...)
	if err != nil {
		return nil, GRPCCallOutput{}, err
	}
	defer cc.Close()

	var descSource grpcurl.DescriptorSource

	if in.UseReflection || len(in.ProtoFiles) == 0 {
		refClient := grpcreflect.NewClient(ctx, reflectpb.NewServerReflectionClient(cc))
		defer refClient.Reset()

		descSource = grpcurl.DescriptorSourceFromServer(ctx, refClient)
	} else {
		descSource, err = grpcurl.DescriptorSourceFromProtoFiles(in.ImportPaths, in.ProtoFiles...)
		if err != nil {
			return nil, GRPCCallOutput{}, err
		}
	}

	var out bytes.Buffer

	formatter := grpcurl.NewJSONFormatter(false, grpcurl.AnyResolverFromDescriptorSource(descSource))
	handler := &grpcurl.DefaultEventHandler{
		Out:            &out,
		Formatter:      formatter,
		VerbosityLevel: 0,
	}

	parser := grpcurl.NewJSONRequestParser(bytes.NewReader(in.RequestJSON), descSource)

	headers := make([]string, 0, len(in.Headers))
	for k, v := range in.Headers {
		headers = append(headers, k+": "+v)
	}

	err = grpcurl.InvokeRPC(
		ctx,
		descSource,
		cc,
		in.Method,
		headers,
		handler,
		parser.Next,
	)
	if err != nil {
		return nil, GRPCCallOutput{}, err
	}

	return nil, GRPCCallOutput{
		ResponseJSON: strings.TrimSpace(out.String()),
	}, nil
}

func main() {
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "grpc-mcp",
			Version: "0.1.0",
		},
		nil,
	)

	mcp.AddTool(
		server,
		&mcp.Tool{
			Name:        "grpc_call",
			Description: "Call a unary gRPC method using JSON input. Supports server reflection or local .proto files.",
		},
		grpcCall,
	)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
