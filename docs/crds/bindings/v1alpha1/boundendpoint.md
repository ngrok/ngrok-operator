# API Reference

Packages:

- [bindings.k8s.ngrok.com/v1alpha1](#bindingsk8sngrokcomv1alpha1)

# bindings.k8s.ngrok.com/v1alpha1

Resource Types:

- [BoundEndpoint](#boundendpoint)




## BoundEndpoint
<sup><sup>[↩ Parent](#bindingsk8sngrokcomv1alpha1 )</sup></sup>






BoundEndpoint is the Schema for the boundendpoints API

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
      <td>bindings.k8s.ngrok.com/v1alpha1</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b>kind</b></td>
      <td>string</td>
      <td>BoundEndpoint</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b><a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#objectmeta-v1-meta">metadata</a></b></td>
      <td>object</td>
      <td>Refer to the Kubernetes API documentation for the fields of the `metadata` field.</td>
      <td>true</td>
      </tr><tr>
        <td><b><a href="#boundendpointspec">spec</a></b></td>
        <td>object</td>
        <td>
          BoundEndpointSpec defines the desired state of BoundEndpoint<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#boundendpointstatus">status</a></b></td>
        <td>object</td>
        <td>
          BoundEndpointStatus defines the observed state of BoundEndpoint<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BoundEndpoint.spec
<sup><sup>[↩ Parent](#boundendpoint)</sup></sup>



BoundEndpointSpec defines the desired state of BoundEndpoint

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
        <td><b>endpointURI</b></td>
        <td>string</td>
        <td>
          EndpointURI is the unique identifier
representing the BoundEndpoint + its Endpoints
Format: <scheme>://<service>.<namespace>:<port>


See: https://regex101.com/r/9QkXWl/1<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>port</b></td>
        <td>integer</td>
        <td>
          Port is the Service port this Endpoint uses internally to communicate with its Upstream Service<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>scheme</b></td>
        <td>enum</td>
        <td>
          Scheme is a user-defined field for endpoints that describe how the data packets
are framed by the pod forwarders mTLS connection to the ngrok edge<br/>
          <br/>
            <i>Enum</i>: tcp, http, https, tls<br/>
            <i>Default</i>: https<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b><a href="#boundendpointspectarget">target</a></b></td>
        <td>object</td>
        <td>
          EndpointTarget is the target Service that this Endpoint projects<br/>
        </td>
        <td>true</td>
      </tr></tbody>
</table>


### BoundEndpoint.spec.target
<sup><sup>[↩ Parent](#boundendpointspec)</sup></sup>



EndpointTarget is the target Service that this Endpoint projects

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
        <td><b>namespace</b></td>
        <td>string</td>
        <td>
          Namespace is the destination Namespace for the Service this Endpoint projects<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>port</b></td>
        <td>integer</td>
        <td>
          Port is the Service targetPort this Endpoint's Target Service uses for requests<br/>
          <br/>
            <i>Format</i>: int32<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>protocol</b></td>
        <td>enum</td>
        <td>
          Protocol is the Service protocol this Endpoint uses<br/>
          <br/>
            <i>Enum</i>: TCP<br/>
            <i>Default</i>: TCP<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>service</b></td>
        <td>string</td>
        <td>
          Service is the name of the Service that this Endpoint projects<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b><a href="#boundendpointspectargetmetadata">metadata</a></b></td>
        <td>object</td>
        <td>
          Metadata is a subset of metav1.ObjectMeta that is added to the Service<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BoundEndpoint.spec.target.metadata
<sup><sup>[↩ Parent](#boundendpointspectarget)</sup></sup>



Metadata is a subset of metav1.ObjectMeta that is added to the Service

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
        <td><b>annotations</b></td>
        <td>map[string]string</td>
        <td>
          Annotations is an unstructured key value map stored with a resource that may be
set by external tools to store and retrieve arbitrary metadata. They are not
queryable and should be preserved when modifying objects.
More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>labels</b></td>
        <td>map[string]string</td>
        <td>
          Map of string keys and values that can be used to organize and categorize
(scope and select) objects. May match selectors of replication controllers
and services.
More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BoundEndpoint.status
<sup><sup>[↩ Parent](#boundendpoint)</sup></sup>



BoundEndpointStatus defines the observed state of BoundEndpoint

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
        <td><b><a href="#boundendpointstatusconditionsindex">conditions</a></b></td>
        <td>[]object</td>
        <td>
          Conditions represent the latest available observations of the BoundEndpoint's state<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#boundendpointstatusendpointsindex">endpoints</a></b></td>
        <td>[]object</td>
        <td>
          Endpoints is the list of ngrok API endpoint references bound to this BoundEndpoint
All endpoints share the same underlying Kubernetes services<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>endpointsSummary</b></td>
        <td>string</td>
        <td>
          EndpointsSummary provides a human-readable count of bound endpoints
Format: "N endpoint" or "N endpoints"
Examples: "1 endpoint", "2 endpoints"<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>hashedName</b></td>
        <td>string</td>
        <td>
          HashName is the hashed output of the TargetService and TargetNamespace for unique identification<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#boundendpointstatustargetserviceref">targetServiceRef</a></b></td>
        <td>object</td>
        <td>
          TargetServiceRef references the created ExternalName Service in the target namespace<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#boundendpointstatusupstreamserviceref">upstreamServiceRef</a></b></td>
        <td>object</td>
        <td>
          UpstreamServiceRef references the created ClusterIP Service pointing to pod forwarders<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BoundEndpoint.status.conditions[index]
<sup><sup>[↩ Parent](#boundendpointstatus)</sup></sup>



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


### BoundEndpoint.status.endpoints[index]
<sup><sup>[↩ Parent](#boundendpointstatus)</sup></sup>



BindingEndpoint is a reference to an Endpoint object in the ngrok API that is attached to the kubernetes operator binding
All endpoints in a BoundEndpoint share the same underlying Kubernetes services

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
        <td><b>id</b></td>
        <td>string</td>
        <td>
          a resource identifier<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>uri</b></td>
        <td>string</td>
        <td>
          a uri for locating a resource<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### BoundEndpoint.status.targetServiceRef
<sup><sup>[↩ Parent](#boundendpointstatus)</sup></sup>



TargetServiceRef references the created ExternalName Service in the target namespace

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


### BoundEndpoint.status.upstreamServiceRef
<sup><sup>[↩ Parent](#boundendpointstatus)</sup></sup>



UpstreamServiceRef references the created ClusterIP Service pointing to pod forwarders

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
