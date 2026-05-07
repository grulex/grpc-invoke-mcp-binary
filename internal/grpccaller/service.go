package grpccaller

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fullstorydev/grpcurl"
	"github.com/jhump/protoreflect/grpcreflect"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

type runtimeConfig struct {
	Target        string
	Plaintext     bool
	TimeoutMS     int
	Headers       map[string]string
	UseReflection bool
	ImportPaths   []string
	ProtoFiles    []string
}

type callRuntime struct {
	ctx        context.Context
	cancel     context.CancelFunc
	cc         *grpc.ClientConn
	descSource grpcurl.DescriptorSource
	headers    []string
	cleanup    func()
}

func (r *callRuntime) Close() {
	if r.cleanup != nil {
		r.cleanup()
	}
	if r.cc != nil {
		_ = r.cc.Close()
	}
	if r.cancel != nil {
		r.cancel()
	}
}

func Call(ctx context.Context, _ *mcp.CallToolRequest, in GRPCCallInput) (*mcp.CallToolResult, GRPCCallOutput, error) {
	if in.Target == "" {
		return nil, GRPCCallOutput{}, fmt.Errorf("target is required")
	}
	if in.Method == "" {
		return nil, GRPCCallOutput{}, fmt.Errorf("method is required")
	}

	requestJSONBytes, err := marshalRequestBody(in.RequestJSON)
	if err != nil {
		return nil, GRPCCallOutput{}, fmt.Errorf("invalid request_json: %w", err)
	}

	runtime, err := prepareRuntime(ctx, runtimeConfig{
		Target:        in.Target,
		Plaintext:     in.Plaintext,
		TimeoutMS:     in.TimeoutMS,
		Headers:       in.Headers,
		UseReflection: in.UseReflection,
		ImportPaths:   in.ImportPaths,
		ProtoFiles:    in.ProtoFiles,
	})
	if err != nil {
		return nil, GRPCCallOutput{}, err
	}
	defer runtime.Close()

	var out bytes.Buffer
	formatter := grpcurl.NewJSONFormatter(false, grpcurl.AnyResolverFromDescriptorSource(runtime.descSource))
	handler := &grpcurl.DefaultEventHandler{
		Out:            &out,
		Formatter:      formatter,
		VerbosityLevel: 0,
	}
	parser := grpcurl.NewJSONRequestParser(
		bytes.NewReader(requestJSONBytes),
		grpcurl.AnyResolverFromDescriptorSource(runtime.descSource),
	)

	err = grpcurl.InvokeRPC(
		runtime.ctx,
		runtime.descSource,
		runtime.cc,
		in.Method,
		runtime.headers,
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

func Describe(ctx context.Context, _ *mcp.CallToolRequest, in GRPCDescribeInput) (*mcp.CallToolResult, GRPCDescribeOutput, error) {
	if in.Target == "" {
		return nil, GRPCDescribeOutput{}, fmt.Errorf("target is required")
	}

	runtime, err := prepareRuntime(ctx, runtimeConfig{
		Target:        in.Target,
		Plaintext:     in.Plaintext,
		TimeoutMS:     in.TimeoutMS,
		Headers:       in.Headers,
		UseReflection: in.UseReflection,
		ImportPaths:   in.ImportPaths,
		ProtoFiles:    in.ProtoFiles,
	})
	if err != nil {
		return nil, GRPCDescribeOutput{}, err
	}
	defer runtime.Close()

	symbols := make([]string, 0, 1)
	if in.Symbol != "" {
		symbols = append(symbols, strings.TrimPrefix(in.Symbol, "."))
	} else {
		symbols, err = grpcurl.ListServices(runtime.descSource)
		if err != nil {
			return nil, GRPCDescribeOutput{}, err
		}
	}

	if len(symbols) == 0 {
		return nil, GRPCDescribeOutput{
			Description: "(No services)",
			Symbols:     symbols,
		}, nil
	}

	var builder strings.Builder
	for i, symbol := range symbols {
		dsc, err := runtime.descSource.FindSymbol(symbol)
		if err != nil {
			return nil, GRPCDescribeOutput{}, fmt.Errorf("failed to resolve symbol %q: %w", symbol, err)
		}
		txt, err := grpcurl.GetDescriptorText(dsc, runtime.descSource)
		if err != nil {
			return nil, GRPCDescribeOutput{}, fmt.Errorf("failed to describe symbol %q: %w", symbol, err)
		}

		if i > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(dsc.GetFullyQualifiedName())
		builder.WriteString(":\n")
		builder.WriteString(strings.TrimSpace(txt))
	}

	return nil, GRPCDescribeOutput{
		Description: builder.String(),
		Symbols:     symbols,
	}, nil
}

func timeoutFromMilliseconds(timeoutMS int) time.Duration {
	timeout := 10 * time.Second
	if timeoutMS > 0 {
		timeout = time.Duration(timeoutMS) * time.Millisecond
	}
	return timeout
}

func dialTarget(target string, plaintext bool) (*grpc.ClientConn, error) {
	var dialOpts []grpc.DialOption
	if plaintext {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(nil, "")))
	}
	return grpc.NewClient(target, dialOpts...)
}

func prepareRuntime(parent context.Context, cfg runtimeConfig) (*callRuntime, error) {
	ctx, cancel := context.WithTimeout(parent, timeoutFromMilliseconds(cfg.TimeoutMS))

	cc, err := dialTarget(cfg.Target, cfg.Plaintext)
	if err != nil {
		cancel()
		return nil, err
	}

	headers := toHeaderList(cfg.Headers)
	descSource, cleanup, err := buildDescriptorSource(
		contextWithMetadata(ctx, headers),
		cc,
		cfg.UseReflection,
		cfg.ImportPaths,
		cfg.ProtoFiles,
	)
	if err != nil {
		_ = cc.Close()
		cancel()
		return nil, err
	}

	return &callRuntime{
		ctx:        ctx,
		cancel:     cancel,
		cc:         cc,
		descSource: descSource,
		headers:    headers,
		cleanup:    cleanup,
	}, nil
}

func buildDescriptorSource(
	ctx context.Context,
	cc *grpc.ClientConn,
	useReflection bool,
	importPaths []string,
	protoFiles []string,
) (grpcurl.DescriptorSource, func(), error) {
	if useReflection || len(protoFiles) == 0 {
		refClient := grpcreflect.NewClientAuto(ctx, cc)
		cleanup := func() { refClient.Reset() }
		return grpcurl.DescriptorSourceFromServer(ctx, refClient), cleanup, nil
	}

	descSource, err := grpcurl.DescriptorSourceFromProtoFiles(importPaths, protoFiles...)
	if err != nil {
		return nil, func() {}, err
	}
	return descSource, func() {}, nil
}

func toHeaderList(headers map[string]string) []string {
	out := make([]string, 0, len(headers))
	for k, v := range headers {
		out = append(out, k+": "+v)
	}
	return out
}

func contextWithMetadata(ctx context.Context, headers []string) context.Context {
	md := grpcurl.MetadataFromHeaders(headers)
	if len(md) == 0 {
		return ctx
	}
	return metadata.NewOutgoingContext(ctx, md)
}
