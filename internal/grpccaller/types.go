package grpccaller

import "encoding/json"

type GRPCCallInput struct {
	Target        string            `json:"target" jsonschema:"host:port, for example localhost:50051"`
	Method        string            `json:"method" jsonschema:"full method, for example helloworld.Greeter/SayHello"`
	RequestJSON   map[string]any    `json:"request_json,omitempty" jsonschema:"JSON request body"`
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

type GRPCDescribeInput struct {
	Target        string            `json:"target" jsonschema:"host:port, for example localhost:50051"`
	Symbol        string            `json:"symbol,omitempty" jsonschema:"fully-qualified symbol to describe, for example helloworld.Greeter/SayHello or package.Service"`
	Plaintext     bool              `json:"plaintext" jsonschema:"use plaintext HTTP/2 instead of TLS"`
	UseReflection bool              `json:"use_reflection" jsonschema:"use gRPC server reflection"`
	ProtoFiles    []string          `json:"proto_files,omitempty" jsonschema:"paths to .proto files"`
	ImportPaths   []string          `json:"import_paths,omitempty" jsonschema:"proto import paths"`
	Headers       map[string]string `json:"headers,omitempty" jsonschema:"request metadata headers"`
	TimeoutMS     int               `json:"timeout_ms,omitempty" jsonschema:"timeout in milliseconds"`
}

type GRPCDescribeOutput struct {
	Description string   `json:"description"`
	Symbols     []string `json:"symbols"`
}

func marshalRequestBody(body map[string]any) (json.RawMessage, error) {
	if body == nil {
		body = map[string]any{}
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return data, nil
}
