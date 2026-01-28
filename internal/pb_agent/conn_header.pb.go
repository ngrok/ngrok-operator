// TODO: pull this from ngrok-go once it is available

package pb_agent

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
	unsafe "unsafe"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type ConnRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Host          string                 `protobuf:"bytes,1,opt,name=host,proto3" json:"host,omitempty"`
	Port          int64                  `protobuf:"varint,2,opt,name=port,proto3" json:"port,omitempty"`
	PodIdentity   *PodIdentity           `protobuf:"bytes,3,opt,name=pod_identity,json=podIdentity,proto3" json:"pod_identity,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ConnRequest) Reset() {
	*x = ConnRequest{}
	mi := &file_agent_conn_header_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ConnRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ConnRequest) ProtoMessage() {}

func (x *ConnRequest) ProtoReflect() protoreflect.Message {
	mi := &file_agent_conn_header_proto_msgTypes[0]
	if x != nil {
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

func (x *ConnRequest) GetPodIdentity() *PodIdentity {
	if x != nil {
		return x.PodIdentity
	}
	return nil
}

type PodIdentity struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Uid           string                 `protobuf:"bytes,1,opt,name=uid,proto3" json:"uid,omitempty"`
	Name          string                 `protobuf:"bytes,2,opt,name=name,proto3" json:"name,omitempty"`
	Namespace     string                 `protobuf:"bytes,3,opt,name=namespace,proto3" json:"namespace,omitempty"`
	Annotations   map[string]string      `protobuf:"bytes,4,rep,name=annotations,proto3" json:"annotations,omitempty" protobuf_key:"bytes,1,opt,name=key" protobuf_val:"bytes,2,opt,name=value"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *PodIdentity) Reset() {
	*x = PodIdentity{}
	mi := &file_agent_conn_header_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *PodIdentity) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PodIdentity) ProtoMessage() {}

func (x *PodIdentity) ProtoReflect() protoreflect.Message {
	mi := &file_agent_conn_header_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PodIdentity.ProtoReflect.Descriptor instead.
func (*PodIdentity) Descriptor() ([]byte, []int) {
	return file_agent_conn_header_proto_rawDescGZIP(), []int{1}
}

func (x *PodIdentity) GetUid() string {
	if x != nil {
		return x.Uid
	}
	return ""
}

func (x *PodIdentity) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *PodIdentity) GetNamespace() string {
	if x != nil {
		return x.Namespace
	}
	return ""
}

func (x *PodIdentity) GetAnnotations() map[string]string {
	if x != nil {
		return x.Annotations
	}
	return nil
}

type ConnResponse struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// only set on success
	EndpointId string `protobuf:"bytes,1,opt,name=endpoint_id,json=endpointId,proto3" json:"endpoint_id,omitempty"`
	Proto      string `protobuf:"bytes,2,opt,name=proto,proto3" json:"proto,omitempty"`
	// only set on error
	ErrorCode     string `protobuf:"bytes,3,opt,name=error_code,json=errorCode,proto3" json:"error_code,omitempty"`
	ErrorMessage  string `protobuf:"bytes,4,opt,name=error_message,json=errorMessage,proto3" json:"error_message,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ConnResponse) Reset() {
	*x = ConnResponse{}
	mi := &file_agent_conn_header_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ConnResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ConnResponse) ProtoMessage() {}

func (x *ConnResponse) ProtoReflect() protoreflect.Message {
	mi := &file_agent_conn_header_proto_msgTypes[2]
	if x != nil {
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
	return file_agent_conn_header_proto_rawDescGZIP(), []int{2}
}

func (x *ConnResponse) GetEndpointId() string {
	if x != nil {
		return x.EndpointId
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

const file_agent_conn_header_proto_rawDesc = "" +
	"\n" +
	"\x17agent/conn_header.proto\x12\x05agent\"l\n" +
	"\vConnRequest\x12\x12\n" +
	"\x04host\x18\x01 \x01(\tR\x04host\x12\x12\n" +
	"\x04port\x18\x02 \x01(\x03R\x04port\x125\n" +
	"\fpod_identity\x18\x03 \x01(\v2\x12.agent.PodIdentityR\vpodIdentity\"\xd8\x01\n" +
	"\vPodIdentity\x12\x10\n" +
	"\x03uid\x18\x01 \x01(\tR\x03uid\x12\x12\n" +
	"\x04name\x18\x02 \x01(\tR\x04name\x12\x1c\n" +
	"\tnamespace\x18\x03 \x01(\tR\tnamespace\x12E\n" +
	"\vannotations\x18\x04 \x03(\v2#.agent.PodIdentity.AnnotationsEntryR\vannotations\x1a>\n" +
	"\x10AnnotationsEntry\x12\x10\n" +
	"\x03key\x18\x01 \x01(\tR\x03key\x12\x14\n" +
	"\x05value\x18\x02 \x01(\tR\x05value:\x028\x01\"\x89\x01\n" +
	"\fConnResponse\x12\x1f\n" +
	"\vendpoint_id\x18\x01 \x01(\tR\n" +
	"endpointId\x12\x14\n" +
	"\x05proto\x18\x02 \x01(\tR\x05proto\x12\x1d\n" +
	"\n" +
	"error_code\x18\x03 \x01(\tR\terrorCode\x12#\n" +
	"\rerror_message\x18\x04 \x01(\tR\ferrorMessageB\x1bZ\x19go.ngrok.com/lib/pb_agentb\x06proto3"

var (
	file_agent_conn_header_proto_rawDescOnce sync.Once
	file_agent_conn_header_proto_rawDescData []byte
)

func file_agent_conn_header_proto_rawDescGZIP() []byte {
	file_agent_conn_header_proto_rawDescOnce.Do(func() {
		file_agent_conn_header_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_agent_conn_header_proto_rawDesc), len(file_agent_conn_header_proto_rawDesc)))
	})
	return file_agent_conn_header_proto_rawDescData
}

var file_agent_conn_header_proto_msgTypes = make([]protoimpl.MessageInfo, 4)
var file_agent_conn_header_proto_goTypes = []any{
	(*ConnRequest)(nil),  // 0: agent.ConnRequest
	(*PodIdentity)(nil),  // 1: agent.PodIdentity
	(*ConnResponse)(nil), // 2: agent.ConnResponse
	nil,                  // 3: agent.PodIdentity.AnnotationsEntry
}
var file_agent_conn_header_proto_depIdxs = []int32{
	1, // 0: agent.ConnRequest.pod_identity:type_name -> agent.PodIdentity
	3, // 1: agent.PodIdentity.annotations:type_name -> agent.PodIdentity.AnnotationsEntry
	2, // [2:2] is the sub-list for method output_type
	2, // [2:2] is the sub-list for method input_type
	2, // [2:2] is the sub-list for extension type_name
	2, // [2:2] is the sub-list for extension extendee
	0, // [0:2] is the sub-list for field type_name
}

func init() { file_agent_conn_header_proto_init() }
func file_agent_conn_header_proto_init() {
	if File_agent_conn_header_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_agent_conn_header_proto_rawDesc), len(file_agent_conn_header_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   4,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_agent_conn_header_proto_goTypes,
		DependencyIndexes: file_agent_conn_header_proto_depIdxs,
		MessageInfos:      file_agent_conn_header_proto_msgTypes,
	}.Build()
	File_agent_conn_header_proto = out.File
	file_agent_conn_header_proto_goTypes = nil
	file_agent_conn_header_proto_depIdxs = nil
}
