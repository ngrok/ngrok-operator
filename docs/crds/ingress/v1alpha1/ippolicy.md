# API Reference

Packages:

- [ingress.k8s.ngrok.com/v1alpha1](#ingressk8sngrokcomv1alpha1)

# ingress.k8s.ngrok.com/v1alpha1

Resource Types:

- [IPPolicy](#ippolicy)




## IPPolicy
<sup><sup>[↩ Parent](#ingressk8sngrokcomv1alpha1 )</sup></sup>






IPPolicy is the Schema for the ippolicies API

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
      <td>ingress.k8s.ngrok.com/v1alpha1</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b>kind</b></td>
      <td>string</td>
      <td>IPPolicy</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b><a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#objectmeta-v1-meta">metadata</a></b></td>
      <td>object</td>
      <td>Refer to the Kubernetes API documentation for the fields of the `metadata` field.</td>
      <td>true</td>
      </tr><tr>
        <td><b><a href="#ippolicyspec">spec</a></b></td>
        <td>object</td>
        <td>
          IPPolicySpec defines the desired state of IPPolicy<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#ippolicystatus">status</a></b></td>
        <td>object</td>
        <td>
          IPPolicyStatus defines the observed state of IPPolicy<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### IPPolicy.spec
<sup><sup>[↩ Parent](#ippolicy)</sup></sup>



IPPolicySpec defines the desired state of IPPolicy

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
        <td><b>description</b></td>
        <td>string</td>
        <td>
          Description is a human-readable description of the object in the ngrok API/Dashboard<br/>
          <br/>
            <i>Default</i>: Created by kubernetes-ingress-controller<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>metadata</b></td>
        <td>string</td>
        <td>
          Metadata is a string of arbitrary data associated with the object in the ngrok API/Dashboard<br/>
          <br/>
            <i>Default</i>: {"owned-by":"kubernetes-ingress-controller"}<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#ippolicyspecrulesindex">rules</a></b></td>
        <td>[]object</td>
        <td>
          Rules is a list of rules that belong to the policy<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### IPPolicy.spec.rules[index]
<sup><sup>[↩ Parent](#ippolicyspec)</sup></sup>





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
        <td><b>action</b></td>
        <td>enum</td>
        <td>
          <br/>
          <br/>
            <i>Enum</i>: allow, deny<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>cidr</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>description</b></td>
        <td>string</td>
        <td>
          Description is a human-readable description of the object in the ngrok API/Dashboard<br/>
          <br/>
            <i>Default</i>: Created by kubernetes-ingress-controller<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>metadata</b></td>
        <td>string</td>
        <td>
          Metadata is a string of arbitrary data associated with the object in the ngrok API/Dashboard<br/>
          <br/>
            <i>Default</i>: {"owned-by":"kubernetes-ingress-controller"}<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### IPPolicy.status
<sup><sup>[↩ Parent](#ippolicy)</sup></sup>



IPPolicyStatus defines the observed state of IPPolicy

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
        <td><b><a href="#ippolicystatusconditionsindex">conditions</a></b></td>
        <td>[]object</td>
        <td>
          Conditions represent the latest available observations of the IP policy's state<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>id</b></td>
        <td>string</td>
        <td>
          INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
Important: Run "make" to regenerate code after modifying this file<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#ippolicystatusrulesindex">rules</a></b></td>
        <td>[]object</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### IPPolicy.status.conditions[index]
<sup><sup>[↩ Parent](#ippolicystatus)</sup></sup>



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


### IPPolicy.status.rules[index]
<sup><sup>[↩ Parent](#ippolicystatus)</sup></sup>





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
        <td><b>action</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>cidr</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>id</b></td>
        <td>string</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>
