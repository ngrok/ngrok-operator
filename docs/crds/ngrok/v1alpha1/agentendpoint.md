# API Reference

Packages:

- [ngrok.k8s.ngrok.com/v1alpha1](#ngrokk8sngrokcomv1alpha1)

# ngrok.k8s.ngrok.com/v1alpha1

Resource Types:

- [AgentEndpoint](#agentendpoint)




## AgentEndpoint
<sup><sup>[↩ Parent](#ngrokk8sngrokcomv1alpha1 )</sup></sup>






AgentEndpoint is the Schema for the agentendpoints API

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
      <td><b>apiVersion</b></td>
      <td>string</td>
      <td>ngrok.k8s.ngrok.com/v1alpha1</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b>kind</b></td>
      <td>string</td>
      <td>AgentEndpoint</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b><a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#objectmeta-v1-meta">metadata</a></b></td>
      <td>object</td>
      <td>Refer to the Kubernetes API documentation for the fields of the `metadata` field.</td>
      <td>true</td>
      </tr><tr>
        <td><b><a href="#agentendpointspec">spec</a></b></td>
        <td>object</td>
        <td>
          AgentEndpointSpec defines the desired state of an AgentEndpoint<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#agentendpointstatus">status</a></b></td>
        <td>object</td>
        <td>
          AgentEndpointStatus defines the observed state of an AgentEndpoint<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### AgentEndpoint.spec
<sup><sup>[↩ Parent](#agentendpoint)</sup></sup>



AgentEndpointSpec defines the desired state of an AgentEndpoint

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b><a href="#agentendpointspecupstream">upstream</a></b></td>
        <td>object</td>
        <td>
          Defines the destination for traffic to this AgentEndpoint<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>url</b></td>
        <td>string</td>
        <td>
          The unique URL for this agent endpoint. This URL is the public address. The following formats are accepted
Domain - example.org
    When using the domain format you are only defining the domain. The scheme and port will be inferred.
Origin - https://example.ngrok.app or https://example.ngrok.app:443 or tcp://1.tcp.ngrok.io:12345 or tls://example.ngrok.app
    When using the origin format you are defining the protocol, domain and port. HTTP endpoints accept ports 80 or 443 with respective protocol.
Scheme (shorthand) - https:// or tcp:// or tls:// or http://
    When using scheme you are defining the protocol and will receive back a randomly assigned ngrok address.
Empty - ``
    When empty your endpoint will default to be https and receive back a randomly assigned ngrok address.
Internal - some.domain.internal
    When ending your url with .internal, an internal endpoint will be created. nternal Endpoints cannot be accessed directly, but rather
    can only be accessed using the forward-internal traffic policy action.<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>bindings</b></td>
        <td>[]string</td>
        <td>
          List of Binding IDs to associate with the endpoint
Accepted values are "public", "internal", or "kubernetes"<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#agentendpointspecclientcertificaterefsindex">clientCertificateRefs</a></b></td>
        <td>[]object</td>
        <td>
          List of client certificates to present to the upstream when performing a TLS handshake<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>description</b></td>
        <td>string</td>
        <td>
          Human-readable description of this agent endpoint<br/>
          <br/>
            <i>Default</i>: Created by the ngrok-operator<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>metadata</b></td>
        <td>string</td>
        <td>
          String of arbitrary data associated with the object in the ngrok API/Dashboard<br/>
          <br/>
            <i>Default</i>: {"owned-by":"ngrok-operator"}<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#agentendpointspectrafficpolicy">trafficPolicy</a></b></td>
        <td>object</td>
        <td>
          Allows configuring a TrafficPolicy to be used with this AgentEndpoint
When configured, the traffic policy is provided inline or as a reference to an NgrokTrafficPolicy resource<br/>
          <br/>
            <i>Validations</i>:<li>has(self.inline) || has(self.targetRef): targetRef or inline must be provided to trafficPolicy</li><li>has(self.inline) != has(self.targetRef): Only one of inline and targetRef can be configured for trafficPolicy</li>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### AgentEndpoint.spec.upstream
<sup><sup>[↩ Parent](#agentendpointspec)</sup></sup>



Defines the destination for traffic to this AgentEndpoint

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>url</b></td>
        <td>string</td>
        <td>
          The local or remote address you would like to incoming traffic to be forwarded to. Accepted formats are:
Origin - https://example.org or http://example.org:80 or tcp://127.0.0.1:80
    When using the origin format you are defining the protocol, domain and port.
        When no port is present and scheme is https or http the port will be inferred.
            For https port will be443.
            For http port will be 80.
Domain - example.org
    This is only allowed for https and http endpoints.
        For tcp and tls endpoints host and port is required.
    When using the domain format you are only defining the host.
        Scheme will default to http.
        Port will default to 80.
Scheme (shorthand) - https://
    This only works for https and http.
        For tcp and tls host and port is required.
    When using scheme you are defining the protocol and the port will be inferred on the local host.
        For https port will be443.
        For http port will be 80.
        Host will be localhost.
Port (shorthand) - 8080
    When using port you are defining the port on the local host that will receive traffic.
        Scheme will default to http.
        Host will default to localhost.<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>protocol</b></td>
        <td>enum</td>
        <td>
          Specifies the protocol to use when connecting to the upstream. Currently only http1 and http2 are supported
with prior knowledge (defaulting to http1). alpn negotiation is not currently supported.<br/>
          <br/>
            <i>Enum</i>: http1, http2<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>proxyProtocolVersion</b></td>
        <td>enum</td>
        <td>
          Optionally specify the version of proxy protocol to use if the upstream requires it<br/>
          <br/>
            <i>Enum</i>: 1, 2<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### AgentEndpoint.spec.clientCertificateRefs[index]
<sup><sup>[↩ Parent](#agentendpointspec)</sup></sup>





<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>name</b></td>
        <td>string</td>
        <td>
          The name of the Kubernetes resource being referenced<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>namespace</b></td>
        <td>string</td>
        <td>
          The namespace of the Kubernetes resource being referenced<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### AgentEndpoint.spec.trafficPolicy
<sup><sup>[↩ Parent](#agentendpointspec)</sup></sup>



Allows configuring a TrafficPolicy to be used with this AgentEndpoint
When configured, the traffic policy is provided inline or as a reference to an NgrokTrafficPolicy resource

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>inline</b></td>
        <td>object</td>
        <td>
          Inline definition of a TrafficPolicy to attach to the agent Endpoint
The raw JSON-encoded policy that was applied to the ngrok API<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#agentendpointspectrafficpolicytargetref">targetRef</a></b></td>
        <td>object</td>
        <td>
          Reference to a TrafficPolicy resource to attach to the Agent Endpoint<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### AgentEndpoint.spec.trafficPolicy.targetRef
<sup><sup>[↩ Parent](#agentendpointspectrafficpolicy)</sup></sup>



Reference to a TrafficPolicy resource to attach to the Agent Endpoint

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>name</b></td>
        <td>string</td>
        <td>
          The name of the Kubernetes resource being referenced<br/>
        </td>
        <td>true</td>
      </tr></tbody>
</table>


### AgentEndpoint.status
<sup><sup>[↩ Parent](#agentendpoint)</sup></sup>



AgentEndpointStatus defines the observed state of an AgentEndpoint

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>assignedURL</b></td>
        <td>string</td>
        <td>
          The assigned URL. This will either be the user-supplied url, or the generated assigned url
depending on the configuration of spec.url<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#agentendpointstatusconditionsindex">conditions</a></b></td>
        <td>[]object</td>
        <td>
          Conditions describe the current conditions of the AgentEndpoint.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#agentendpointstatusdomainref">domainRef</a></b></td>
        <td>object</td>
        <td>
          DomainRef is a reference to the Domain resource associated with this endpoint.
For internal endpoints, this will be nil.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>trafficPolicy</b></td>
        <td>string</td>
        <td>
          Identifies any traffic policies attached to the AgentEndpoint ("inline", "none", or reference name).<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### AgentEndpoint.status.conditions[index]
<sup><sup>[↩ Parent](#agentendpointstatus)</sup></sup>



Condition contains details for one aspect of the current state of this API Resource.
---
This struct is intended for direct use as an array at the field path .status.conditions.  For example,


	type FooStatus struct{
	    // Represents the observations of a foo's current state.
	    // Known .status.conditions.type are: "Available", "Progressing", and "Degraded"
	    // +patchMergeKey=type
	    // +patchStrategy=merge
	    // +listType=map
	    // +listMapKey=type
	    Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`


	    // other fields
	}

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>lastTransitionTime</b></td>
        <td>string</td>
        <td>
          lastTransitionTime is the last time the condition transitioned from one status to another.
This should be when the underlying condition changed.  If that is not known, then using the time when the API field changed is acceptable.<br/>
          <br/>
            <i>Format</i>: date-time<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>message</b></td>
        <td>string</td>
        <td>
          message is a human readable message indicating details about the transition.
This may be an empty string.<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>reason</b></td>
        <td>string</td>
        <td>
          reason contains a programmatic identifier indicating the reason for the condition's last transition.
Producers of specific condition types may define expected values and meanings for this field,
and whether the values are considered a guaranteed API.
The value should be a CamelCase string.
This field may not be empty.<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>status</b></td>
        <td>enum</td>
        <td>
          status of the condition, one of True, False, Unknown.<br/>
          <br/>
            <i>Enum</i>: True, False, Unknown<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>type</b></td>
        <td>string</td>
        <td>
          type of condition in CamelCase or in foo.example.com/CamelCase.
---
Many .condition.type values are consistent across resources like Available, but because arbitrary conditions can be
useful (see .node.status.conditions), the ability to deconflict is important.
The regex it matches is (dns1123SubdomainFmt/)?(qualifiedNameFmt)<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>observedGeneration</b></td>
        <td>integer</td>
        <td>
          observedGeneration represents the .metadata.generation that the condition was set based upon.
For instance, if .metadata.generation is currently 12, but the .status.conditions[x].observedGeneration is 9, the condition is out of date
with respect to the current state of the instance.<br/>
          <br/>
            <i>Format</i>: int64<br/>
            <i>Minimum</i>: 0<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### AgentEndpoint.status.domainRef
<sup><sup>[↩ Parent](#agentendpointstatus)</sup></sup>



DomainRef is a reference to the Domain resource associated with this endpoint.
For internal endpoints, this will be nil.

<table>
    <thead>
        <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Description</th>
            <th>Required</th>
        </tr>
    </thead>
    <tbody><tr>
        <td><b>name</b></td>
        <td>string</td>
        <td>
          The name of the Kubernetes resource being referenced<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>namespace</b></td>
        <td>string</td>
        <td>
          The namespace of the Kubernetes resource being referenced<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>
