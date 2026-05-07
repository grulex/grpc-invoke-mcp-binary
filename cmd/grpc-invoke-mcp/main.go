package main

import (
	"context"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"grpc-invoke-mcp/internal/grpccaller"
)

func main() {
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "grpc-invoke-mcp",
			Title:   "gRPC Invoke MCP Server (Call & Describe via Reflection or Proto Files)",
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
		grpccaller.Call,
	)

	mcp.AddTool(
		server,
		&mcp.Tool{
			Name:        "grpc_describe",
			Description: "Describe gRPC services, methods, or message symbols using server reflection or local .proto files.",
		},
		grpccaller.Describe,
	)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
