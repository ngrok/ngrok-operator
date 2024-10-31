// TODO: pull this from ngrok-go once it is available

package pb_agent

import (
	reflect "reflect"
	sync "sync"

	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type ConnRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Host string `protobuf:"bytes,1,opt,name=host,proto3" json:"host,omitempty"`
	Port int64  `protobuf:"varint,2,opt,name=port,proto3" json:"port,omitempty"`
}

func (x *ConnRequest) Reset() {
	*x = ConnRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_agent_conn_header_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ConnRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ConnRequest) ProtoMessage() {}

func (x *ConnRequest) ProtoReflect() protoreflect.Message {
	mi := &file_agent_conn_header_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ConnRequest.ProtoReflect.Descriptor instead.
func (*ConnRequest) Descriptor() ([]byte, []int) {
	return file_agent_conn_header_proto_rawDescGZIP(), []int{0}
}

func (x *ConnRequest) GetHost() string {
	if x != nil {
		return x.Host
	}
	return ""
}

func (x *ConnRequest) GetPort() int64 {
	if x != nil {
		return x.Port
	}
	return 0
}

type ConnResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// only set on success
	EndpointID string `protobuf:"bytes,1,opt,name=endpoint_id,json=endpointId,proto3" json:"endpoint_id,omitempty"`
	Proto      string `protobuf:"bytes,2,opt,name=proto,proto3" json:"proto,omitempty"`
	// only set on error
	ErrorCode    string `protobuf:"bytes,3,opt,name=error_code,json=errorCode,proto3" json:"error_code,omitempty"`
	ErrorMessage string `protobuf:"bytes,4,opt,name=error_message,json=errorMessage,proto3" json:"error_message,omitempty"`
}

func (x *ConnResponse) Reset() {
	*x = ConnResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_agent_conn_header_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ConnResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ConnResponse) ProtoMessage() {}

func (x *ConnResponse) ProtoReflect() protoreflect.Message {
	mi := &file_agent_conn_header_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ConnResponse.ProtoReflect.Descriptor instead.
func (*ConnResponse) Descriptor() ([]byte, []int) {
	return file_agent_conn_header_proto_rawDescGZIP(), []int{1}
}

func (x *ConnResponse) GetEndpointID() string {
	if x != nil {
		return x.EndpointID
	}
	return ""
}

func (x *ConnResponse) GetProto() string {
	if x != nil {
		return x.Proto
	}
	return ""
}

func (x *ConnResponse) GetErrorCode() string {
	if x != nil {
		return x.ErrorCode
	}
	return ""
}

func (x *ConnResponse) GetErrorMessage() string {
	if x != nil {
		return x.ErrorMessage
	}
	return ""
}

var File_agent_conn_header_proto protoreflect.FileDescriptor

var file_agent_conn_header_proto_rawDesc = []byte{
	0x0a, 0x17, 0x61, 0x67, 0x65, 0x6e, 0x74, 0x2f, 0x63, 0x6f, 0x6e, 0x6e, 0x5f, 0x68, 0x65, 0x61,
	0x64, 0x65, 0x72, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x05, 0x61, 0x67, 0x65, 0x6e, 0x74,
	0x22, 0x35, 0x0a, 0x0b, 0x43, 0x6f, 0x6e, 0x6e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12,
	0x12, 0x0a, 0x04, 0x68, 0x6f, 0x73, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x68,
	0x6f, 0x73, 0x74, 0x12, 0x12, 0x0a, 0x04, 0x70, 0x6f, 0x72, 0x74, 0x18, 0x02, 0x20, 0x01, 0x28,
	0x03, 0x52, 0x04, 0x70, 0x6f, 0x72, 0x74, 0x22, 0x89, 0x01, 0x0a, 0x0c, 0x43, 0x6f, 0x6e, 0x6e,
	0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x1f, 0x0a, 0x0b, 0x65, 0x6e, 0x64, 0x70,
	0x6f, 0x69, 0x6e, 0x74, 0x5f, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0a, 0x65,
	0x6e, 0x64, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x49, 0x64, 0x12, 0x14, 0x0a, 0x05, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12,
	0x1d, 0x0a, 0x0a, 0x65, 0x72, 0x72, 0x6f, 0x72, 0x5f, 0x63, 0x6f, 0x64, 0x65, 0x18, 0x03, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x09, 0x65, 0x72, 0x72, 0x6f, 0x72, 0x43, 0x6f, 0x64, 0x65, 0x12, 0x23,
	0x0a, 0x0d, 0x65, 0x72, 0x72, 0x6f, 0x72, 0x5f, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x18,
	0x04, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0c, 0x65, 0x72, 0x72, 0x6f, 0x72, 0x4d, 0x65, 0x73, 0x73,
	0x61, 0x67, 0x65, 0x42, 0x1b, 0x5a, 0x19, 0x67, 0x6f, 0x2e, 0x6e, 0x67, 0x72, 0x6f, 0x6b, 0x2e,
	0x63, 0x6f, 0x6d, 0x2f, 0x6c, 0x69, 0x62, 0x2f, 0x70, 0x62, 0x5f, 0x61, 0x67, 0x65, 0x6e, 0x74,
	0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_agent_conn_header_proto_rawDescOnce sync.Once
	file_agent_conn_header_proto_rawDescData = file_agent_conn_header_proto_rawDesc
)

func file_agent_conn_header_proto_rawDescGZIP() []byte {
	file_agent_conn_header_proto_rawDescOnce.Do(func() {
		file_agent_conn_header_proto_rawDescData = protoimpl.X.CompressGZIP(file_agent_conn_header_proto_rawDescData)
	})
	return file_agent_conn_header_proto_rawDescData
}

var file_agent_conn_header_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_agent_conn_header_proto_goTypes = []interface{}{
	(*ConnRequest)(nil),  // 0: agent.ConnRequest
	(*ConnResponse)(nil), // 1: agent.ConnResponse
}
var file_agent_conn_header_proto_depIdxs = []int32{
	0, // [0:0] is the sub-list for method output_type
	0, // [0:0] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_agent_conn_header_proto_init() }
func file_agent_conn_header_proto_init() {
	if File_agent_conn_header_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_agent_conn_header_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ConnRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_agent_conn_header_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ConnResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_agent_conn_header_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_agent_conn_header_proto_goTypes,
		DependencyIndexes: file_agent_conn_header_proto_depIdxs,
		MessageInfos:      file_agent_conn_header_proto_msgTypes,
	}.Build()
	File_agent_conn_header_proto = out.File
	file_agent_conn_header_proto_rawDesc = nil
	file_agent_conn_header_proto_goTypes = nil
	file_agent_conn_header_proto_depIdxs = nil
}
