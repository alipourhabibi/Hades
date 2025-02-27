package sdk

import (
	"context"
	"io"
	"os/exec"

	"github.com/bufbuild/protocompile"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

type Generator struct {
}

func (g *Generator) Generate(ctx context.Context, protoFiles map[string]string) (map[string]string, error) {

	// // SDK
	// generateFiles := map[string]string{}
	// for _, v := range files {
	// 	if strings.HasSuffix(v.Path, ".proto") {
	// 		generateFiles[v.Path] = string(v.Content)
	// 	}
	// }
	// gens, err := s.generator.Generate(ctx, generateFiles)
	// if err != nil {
	// 	return nil, err
	// }
	// // TODO send this gens to gitaly
	// _ = gens
	// //s.logger.Debug("Generated", "files", gens)

	resolver := &protocompile.SourceResolver{
		Accessor: protocompile.SourceAccessorFromMap(protoFiles),
	}
	compiler := protocompile.Compiler{Resolver: resolver}
	keys := make([]string, 0, len(protoFiles))
	for k := range protoFiles {
		keys = append(keys, k)
	}
	descriptors, err := compiler.Compile(ctx, keys...)
	if err != nil {
		return nil, err
	}

	// Convert descriptors to FileDescriptorProto
	var fileDescProtos []*descriptorpb.FileDescriptorProto
	for _, fd := range descriptors {
		fdProto := protodesc.ToFileDescriptorProto(fd)
		fileDescProtos = append(fileDescProtos, fdProto)
	}
	p := "paths=source_relative"
	// Build CodeGeneratorRequest
	req := &pluginpb.CodeGeneratorRequest{
		FileToGenerate: keys,
		ProtoFile:      fileDescProtos,
		Parameter:      &p, // Optional: Ensure output paths match the source structure
	}

	// Serialize the request
	reqData, err := proto.Marshal(req)
	if err != nil {
		return nil, err
	}

	// Execute protoc-gen-go
	cmd := exec.Command("protoc-gen-go")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	// Write request to stdin
	_, err = stdin.Write(reqData)
	if err != nil {
		return nil, err
	}
	stdin.Close()

	// Read plugin output
	respData, err := io.ReadAll(stdout)
	if err != nil {
		return nil, err
	}
	if err := cmd.Wait(); err != nil {
		return nil, err
	}

	// Unmarshal response
	var resp pluginpb.CodeGeneratorResponse
	if err := proto.Unmarshal(respData, &resp); err != nil {
		return nil, err
	}

	outPutFiles := map[string]string{}

	// Write generated Go code to file
	for _, file := range resp.File {

		outPutFiles[file.GetName()] = file.GetContent()
	}

	return outPutFiles, nil
}
