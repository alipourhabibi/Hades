// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.3
// 	protoc        (unknown)
// source: api/registry/v1/download.proto

package registryv1

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type File struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Path          string                 `protobuf:"bytes,1,opt,name=path,proto3" json:"path,omitempty"`
	Content       []byte                 `protobuf:"bytes,2,opt,name=content,proto3" json:"content,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *File) Reset() {
	*x = File{}
	mi := &file_api_registry_v1_download_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *File) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*File) ProtoMessage() {}

func (x *File) ProtoReflect() protoreflect.Message {
	mi := &file_api_registry_v1_download_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use File.ProtoReflect.Descriptor instead.
func (*File) Descriptor() ([]byte, []int) {
	return file_api_registry_v1_download_proto_rawDescGZIP(), []int{0}
}

func (x *File) GetPath() string {
	if x != nil {
		return x.Path
	}
	return ""
}

func (x *File) GetContent() []byte {
	if x != nil {
		return x.Content
	}
	return nil
}

type DownloadResponseContent struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Commit        *Commit                `protobuf:"bytes,1,opt,name=commit,proto3" json:"commit,omitempty"`
	Files         []*File                `protobuf:"bytes,2,rep,name=files,proto3" json:"files,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *DownloadResponseContent) Reset() {
	*x = DownloadResponseContent{}
	mi := &file_api_registry_v1_download_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *DownloadResponseContent) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*DownloadResponseContent) ProtoMessage() {}

func (x *DownloadResponseContent) ProtoReflect() protoreflect.Message {
	mi := &file_api_registry_v1_download_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use DownloadResponseContent.ProtoReflect.Descriptor instead.
func (*DownloadResponseContent) Descriptor() ([]byte, []int) {
	return file_api_registry_v1_download_proto_rawDescGZIP(), []int{1}
}

func (x *DownloadResponseContent) GetCommit() *Commit {
	if x != nil {
		return x.Commit
	}
	return nil
}

func (x *DownloadResponseContent) GetFiles() []*File {
	if x != nil {
		return x.Files
	}
	return nil
}

type DownloadResponse struct {
	state         protoimpl.MessageState     `protogen:"open.v1"`
	Contents      []*DownloadResponseContent `protobuf:"bytes,1,rep,name=contents,proto3" json:"contents,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *DownloadResponse) Reset() {
	*x = DownloadResponse{}
	mi := &file_api_registry_v1_download_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *DownloadResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*DownloadResponse) ProtoMessage() {}

func (x *DownloadResponse) ProtoReflect() protoreflect.Message {
	mi := &file_api_registry_v1_download_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use DownloadResponse.ProtoReflect.Descriptor instead.
func (*DownloadResponse) Descriptor() ([]byte, []int) {
	return file_api_registry_v1_download_proto_rawDescGZIP(), []int{2}
}

func (x *DownloadResponse) GetContents() []*DownloadResponseContent {
	if x != nil {
		return x.Contents
	}
	return nil
}

var File_api_registry_v1_download_proto protoreflect.FileDescriptor

var file_api_registry_v1_download_proto_rawDesc = []byte{
	0x0a, 0x1e, 0x61, 0x70, 0x69, 0x2f, 0x72, 0x65, 0x67, 0x69, 0x73, 0x74, 0x72, 0x79, 0x2f, 0x76,
	0x31, 0x2f, 0x64, 0x6f, 0x77, 0x6e, 0x6c, 0x6f, 0x61, 0x64, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x12, 0x15, 0x68, 0x61, 0x64, 0x65, 0x73, 0x2e, 0x61, 0x70, 0x69, 0x2e, 0x72, 0x65, 0x67, 0x69,
	0x73, 0x74, 0x72, 0x79, 0x2e, 0x76, 0x31, 0x1a, 0x1c, 0x61, 0x70, 0x69, 0x2f, 0x72, 0x65, 0x67,
	0x69, 0x73, 0x74, 0x72, 0x79, 0x2f, 0x76, 0x31, 0x2f, 0x63, 0x6f, 0x6d, 0x6d, 0x69, 0x74, 0x2e,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x34, 0x0a, 0x04, 0x46, 0x69, 0x6c, 0x65, 0x12, 0x12, 0x0a,
	0x04, 0x70, 0x61, 0x74, 0x68, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x70, 0x61, 0x74,
	0x68, 0x12, 0x18, 0x0a, 0x07, 0x63, 0x6f, 0x6e, 0x74, 0x65, 0x6e, 0x74, 0x18, 0x02, 0x20, 0x01,
	0x28, 0x0c, 0x52, 0x07, 0x63, 0x6f, 0x6e, 0x74, 0x65, 0x6e, 0x74, 0x22, 0x83, 0x01, 0x0a, 0x17,
	0x44, 0x6f, 0x77, 0x6e, 0x6c, 0x6f, 0x61, 0x64, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65,
	0x43, 0x6f, 0x6e, 0x74, 0x65, 0x6e, 0x74, 0x12, 0x35, 0x0a, 0x06, 0x63, 0x6f, 0x6d, 0x6d, 0x69,
	0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1d, 0x2e, 0x68, 0x61, 0x64, 0x65, 0x73, 0x2e,
	0x61, 0x70, 0x69, 0x2e, 0x72, 0x65, 0x67, 0x69, 0x73, 0x74, 0x72, 0x79, 0x2e, 0x76, 0x31, 0x2e,
	0x43, 0x6f, 0x6d, 0x6d, 0x69, 0x74, 0x52, 0x06, 0x63, 0x6f, 0x6d, 0x6d, 0x69, 0x74, 0x12, 0x31,
	0x0a, 0x05, 0x66, 0x69, 0x6c, 0x65, 0x73, 0x18, 0x02, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x1b, 0x2e,
	0x68, 0x61, 0x64, 0x65, 0x73, 0x2e, 0x61, 0x70, 0x69, 0x2e, 0x72, 0x65, 0x67, 0x69, 0x73, 0x74,
	0x72, 0x79, 0x2e, 0x76, 0x31, 0x2e, 0x46, 0x69, 0x6c, 0x65, 0x52, 0x05, 0x66, 0x69, 0x6c, 0x65,
	0x73, 0x22, 0x5e, 0x0a, 0x10, 0x44, 0x6f, 0x77, 0x6e, 0x6c, 0x6f, 0x61, 0x64, 0x52, 0x65, 0x73,
	0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x4a, 0x0a, 0x08, 0x63, 0x6f, 0x6e, 0x74, 0x65, 0x6e, 0x74,
	0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x2e, 0x2e, 0x68, 0x61, 0x64, 0x65, 0x73, 0x2e,
	0x61, 0x70, 0x69, 0x2e, 0x72, 0x65, 0x67, 0x69, 0x73, 0x74, 0x72, 0x79, 0x2e, 0x76, 0x31, 0x2e,
	0x44, 0x6f, 0x77, 0x6e, 0x6c, 0x6f, 0x61, 0x64, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65,
	0x43, 0x6f, 0x6e, 0x74, 0x65, 0x6e, 0x74, 0x52, 0x08, 0x63, 0x6f, 0x6e, 0x74, 0x65, 0x6e, 0x74,
	0x73, 0x42, 0xe4, 0x01, 0x0a, 0x19, 0x63, 0x6f, 0x6d, 0x2e, 0x68, 0x61, 0x64, 0x65, 0x73, 0x2e,
	0x61, 0x70, 0x69, 0x2e, 0x72, 0x65, 0x67, 0x69, 0x73, 0x74, 0x72, 0x79, 0x2e, 0x76, 0x31, 0x42,
	0x0d, 0x44, 0x6f, 0x77, 0x6e, 0x6c, 0x6f, 0x61, 0x64, 0x50, 0x72, 0x6f, 0x74, 0x6f, 0x50, 0x01,
	0x5a, 0x41, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x61, 0x6c, 0x69,
	0x70, 0x6f, 0x75, 0x72, 0x68, 0x61, 0x62, 0x69, 0x62, 0x69, 0x2f, 0x48, 0x61, 0x64, 0x65, 0x73,
	0x2f, 0x61, 0x70, 0x69, 0x2f, 0x67, 0x65, 0x6e, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x72, 0x65, 0x67,
	0x69, 0x73, 0x74, 0x72, 0x79, 0x2f, 0x76, 0x31, 0x3b, 0x72, 0x65, 0x67, 0x69, 0x73, 0x74, 0x72,
	0x79, 0x76, 0x31, 0xa2, 0x02, 0x03, 0x48, 0x41, 0x52, 0xaa, 0x02, 0x15, 0x48, 0x61, 0x64, 0x65,
	0x73, 0x2e, 0x41, 0x70, 0x69, 0x2e, 0x52, 0x65, 0x67, 0x69, 0x73, 0x74, 0x72, 0x79, 0x2e, 0x56,
	0x31, 0xca, 0x02, 0x15, 0x48, 0x61, 0x64, 0x65, 0x73, 0x5c, 0x41, 0x70, 0x69, 0x5c, 0x52, 0x65,
	0x67, 0x69, 0x73, 0x74, 0x72, 0x79, 0x5c, 0x56, 0x31, 0xe2, 0x02, 0x21, 0x48, 0x61, 0x64, 0x65,
	0x73, 0x5c, 0x41, 0x70, 0x69, 0x5c, 0x52, 0x65, 0x67, 0x69, 0x73, 0x74, 0x72, 0x79, 0x5c, 0x56,
	0x31, 0x5c, 0x47, 0x50, 0x42, 0x4d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0xea, 0x02, 0x18,
	0x48, 0x61, 0x64, 0x65, 0x73, 0x3a, 0x3a, 0x41, 0x70, 0x69, 0x3a, 0x3a, 0x52, 0x65, 0x67, 0x69,
	0x73, 0x74, 0x72, 0x79, 0x3a, 0x3a, 0x56, 0x31, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_api_registry_v1_download_proto_rawDescOnce sync.Once
	file_api_registry_v1_download_proto_rawDescData = file_api_registry_v1_download_proto_rawDesc
)

func file_api_registry_v1_download_proto_rawDescGZIP() []byte {
	file_api_registry_v1_download_proto_rawDescOnce.Do(func() {
		file_api_registry_v1_download_proto_rawDescData = protoimpl.X.CompressGZIP(file_api_registry_v1_download_proto_rawDescData)
	})
	return file_api_registry_v1_download_proto_rawDescData
}

var file_api_registry_v1_download_proto_msgTypes = make([]protoimpl.MessageInfo, 3)
var file_api_registry_v1_download_proto_goTypes = []any{
	(*File)(nil),                    // 0: hades.api.registry.v1.File
	(*DownloadResponseContent)(nil), // 1: hades.api.registry.v1.DownloadResponseContent
	(*DownloadResponse)(nil),        // 2: hades.api.registry.v1.DownloadResponse
	(*Commit)(nil),                  // 3: hades.api.registry.v1.Commit
}
var file_api_registry_v1_download_proto_depIdxs = []int32{
	3, // 0: hades.api.registry.v1.DownloadResponseContent.commit:type_name -> hades.api.registry.v1.Commit
	0, // 1: hades.api.registry.v1.DownloadResponseContent.files:type_name -> hades.api.registry.v1.File
	1, // 2: hades.api.registry.v1.DownloadResponse.contents:type_name -> hades.api.registry.v1.DownloadResponseContent
	3, // [3:3] is the sub-list for method output_type
	3, // [3:3] is the sub-list for method input_type
	3, // [3:3] is the sub-list for extension type_name
	3, // [3:3] is the sub-list for extension extendee
	0, // [0:3] is the sub-list for field type_name
}

func init() { file_api_registry_v1_download_proto_init() }
func file_api_registry_v1_download_proto_init() {
	if File_api_registry_v1_download_proto != nil {
		return
	}
	file_api_registry_v1_commit_proto_init()
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_api_registry_v1_download_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   3,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_api_registry_v1_download_proto_goTypes,
		DependencyIndexes: file_api_registry_v1_download_proto_depIdxs,
		MessageInfos:      file_api_registry_v1_download_proto_msgTypes,
	}.Build()
	File_api_registry_v1_download_proto = out.File
	file_api_registry_v1_download_proto_rawDesc = nil
	file_api_registry_v1_download_proto_goTypes = nil
	file_api_registry_v1_download_proto_depIdxs = nil
}
