# API Reference

Packages:

- [ngrok.k8s.ngrok.com/v1alpha1](#ngrokk8sngrokcomv1alpha1)

# ngrok.k8s.ngrok.com/v1alpha1

Resource Types:

- [CloudEndpoint](#cloudendpoint)




## CloudEndpoint
<sup><sup>[↩ Parent](#ngrokk8sngrokcomv1alpha1 )</sup></sup>






CloudEndpoint is the Schema for the cloudendpoints API

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
      <td>CloudEndpoint</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b><a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#objectmeta-v1-meta">metadata</a></b></td>
      <td>object</td>
      <td>Refer to the Kubernetes API documentation for the fields of the `metadata` field.</td>
      <td>true</td>
      </tr><tr>
        <td><b><a href="#cloudendpointspec">spec</a></b></td>
        <td>object</td>
        <td>
          CloudEndpointSpec defines the desired state of CloudEndpoint<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#cloudendpointstatus">status</a></b></td>
        <td>object</td>
        <td>
          CloudEndpointStatus defines the observed state of CloudEndpoint<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### CloudEndpoint.spec
<sup><sup>[↩ Parent](#cloudendpoint)</sup></sup>



CloudEndpointSpec defines the desired state of CloudEndpoint

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
          The unique URL for this cloud endpoint. This URL is the public address. The following formats are accepted
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
          Bindings is the list of Binding IDs to associate with the endpoint
Accepted values are "public", "internal", or "kubernetes"<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>description</b></td>
        <td>string</td>
        <td>
          Human-readable description of this cloud endpoint<br/>
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
        <td><b>poolingEnabled</b></td>
        <td>boolean</td>
        <td>
          Controls whether or not the Cloud Endpoint should allow pooling with other
Cloud Endpoints sharing the same URL. When Cloud Endpoints are pooled, any requests
going to the URL for the pooled endpoint will be distributed among all Cloud Endpoints
in the pool. A URL can only be shared across multiple Cloud Endpoints if they all have pooling enabled.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#cloudendpointspectrafficpolicy">trafficPolicy</a></b></td>
        <td>object</td>
        <td>
          Allows inline definition of a TrafficPolicy object<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>trafficPolicyName</b></td>
        <td>string</td>
        <td>
          Reference to the TrafficPolicy resource to attach to the Cloud Endpoint<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### CloudEndpoint.spec.trafficPolicy
<sup><sup>[↩ Parent](#cloudendpointspec)</sup></sup>



Allows inline definition of a TrafficPolicy object

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
        <td><b>policy</b></td>
        <td>object</td>
        <td>
          The raw json encoded policy that was applied to the ngrok API<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### CloudEndpoint.status
<sup><sup>[↩ Parent](#cloudendpoint)</sup></sup>



CloudEndpointStatus defines the observed state of CloudEndpoint

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
        <td><b><a href="#cloudendpointstatusconditionsindex">conditions</a></b></td>
        <td>[]object</td>
        <td>
          Conditions describe the current conditions of the AgentEndpoint.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#cloudendpointstatusdomain">domain</a></b></td>
        <td>object</td>
        <td>
          Deprecated: This is here for backwards compatibility with the old DomainStatus object.
This will be removed in a future version. Use DomainRef instead.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#cloudendpointstatusdomainref">domainRef</a></b></td>
        <td>object</td>
        <td>
          DomainRef is a reference to the Domain resource associated with this endpoint.
For internal endpoints, this will be nil.<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>id</b></td>
        <td>string</td>
        <td>
          ID is the unique identifier for this endpoint<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### CloudEndpoint.status.conditions[index]
<sup><sup>[↩ Parent](#cloudendpointstatus)</sup></sup>



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


### CloudEndpoint.status.domain
<sup><sup>[↩ Parent](#cloudendpointstatus)</sup></sup>



Deprecated: This is here for backwards compatibility with the old DomainStatus object.
This will be removed in a future version. Use DomainRef instead.

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
        <td><b>cnameTarget</b></td>
        <td>string</td>
        <td>
          CNAMETarget is the CNAME target for the domain<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>domain</b></td>
        <td>string</td>
        <td>
          Domain is the domain that was reserved<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>id</b></td>
        <td>string</td>
        <td>
          ID is the unique identifier of the domain<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>region</b></td>
        <td>string</td>
        <td>
          Region is the region in which the domain was created<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### CloudEndpoint.status.domainRef
<sup><sup>[↩ Parent](#cloudendpointstatus)</sup></sup>



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
