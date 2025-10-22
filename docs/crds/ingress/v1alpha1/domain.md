# API Reference

Packages:

- [ingress.k8s.ngrok.com/v1alpha1](#ingressk8sngrokcomv1alpha1)

# ingress.k8s.ngrok.com/v1alpha1

Resource Types:

- [Domain](#domain)




## Domain
<sup><sup>[↩ Parent](#ingressk8sngrokcomv1alpha1 )</sup></sup>






Domain is the Schema for the domains API

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
      <td>Domain</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b><a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#objectmeta-v1-meta">metadata</a></b></td>
      <td>object</td>
      <td>Refer to the Kubernetes API documentation for the fields of the `metadata` field.</td>
      <td>true</td>
      </tr><tr>
        <td><b><a href="#domainspec">spec</a></b></td>
        <td>object</td>
        <td>
          DomainSpec defines the desired state of Domain<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#domainstatus">status</a></b></td>
        <td>object</td>
        <td>
          DomainStatus defines the observed state of Domain<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### Domain.spec
<sup><sup>[↩ Parent](#domain)</sup></sup>



DomainSpec defines the desired state of Domain

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
        <td><b>domain</b></td>
        <td>string</td>
        <td>
          Domain is the domain name to reserve<br/>
        </td>
        <td>true</td>
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
      </tr><tr>
        <td><b>reclaimPolicy</b></td>
        <td>enum</td>
        <td>
          DomainReclaimPolicy is the policy to use when the domain is deleted<br/>
          <br/>
            <i>Enum</i>: Delete, Retain<br/>
            <i>Default</i>: Delete<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>region</b></td>
        <td>string</td>
        <td>
          Region is the region in which to reserve the domain<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### Domain.status
<sup><sup>[↩ Parent](#domain)</sup></sup>



DomainStatus defines the observed state of Domain

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
        <td><b>acmeChallengeCnameTarget</b></td>
        <td>string</td>
        <td>
          ACMEChallengeCNAMETarget is the CNAME target for ACME challenge (wildcards only)<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#domainstatuscertificate">certificate</a></b></td>
        <td>object</td>
        <td>
          Certificate contains information about the TLS certificate<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#domainstatuscertificatemanagementpolicy">certificateManagementPolicy</a></b></td>
        <td>object</td>
        <td>
          CertificateManagementPolicy contains the certificate management configuration<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#domainstatuscertificatemanagementstatus">certificateManagementStatus</a></b></td>
        <td>object</td>
        <td>
          CertificateManagementStatus contains the certificate management status<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>cnameTarget</b></td>
        <td>string</td>
        <td>
          CNAMETarget is the CNAME target for the domain<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#domainstatusconditionsindex">conditions</a></b></td>
        <td>[]object</td>
        <td>
          Conditions represent the latest available observations of the domain's state<br/>
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


### Domain.status.certificate
<sup><sup>[↩ Parent](#domainstatus)</sup></sup>



Certificate contains information about the TLS certificate

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
          ID is the certificate ID<br/>
        </td>
        <td>true</td>
      </tr></tbody>
</table>


### Domain.status.certificateManagementPolicy
<sup><sup>[↩ Parent](#domainstatus)</sup></sup>



CertificateManagementPolicy contains the certificate management configuration

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
        <td><b>authority</b></td>
        <td>string</td>
        <td>
          Authority is the certificate authority (e.g., "letsencrypt")<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>privateKeyType</b></td>
        <td>string</td>
        <td>
          PrivateKeyType is the private key type (e.g., "ecdsa")<br/>
        </td>
        <td>true</td>
      </tr></tbody>
</table>


### Domain.status.certificateManagementStatus
<sup><sup>[↩ Parent](#domainstatus)</sup></sup>



CertificateManagementStatus contains the certificate management status

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
        <td><b><a href="#domainstatuscertificatemanagementstatusprovisioningjob">provisioningJob</a></b></td>
        <td>object</td>
        <td>
          ProvisioningJob contains information about the current provisioning job<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>renewsAt</b></td>
        <td>string</td>
        <td>
          RenewsAt is when the certificate will be renewed<br/>
          <br/>
            <i>Format</i>: date-time<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### Domain.status.certificateManagementStatus.provisioningJob
<sup><sup>[↩ Parent](#domainstatuscertificatemanagementstatus)</sup></sup>



ProvisioningJob contains information about the current provisioning job

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
        <td><b>errorCode</b></td>
        <td>string</td>
        <td>
          ErrorCode indicates the type of error (e.g., "DNS_ERROR")<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>message</b></td>
        <td>string</td>
        <td>
          Message is a human-readable description of the current status<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>retriesAt</b></td>
        <td>string</td>
        <td>
          RetriesAt is when the provisioning job will be retried<br/>
          <br/>
            <i>Format</i>: date-time<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>startedAt</b></td>
        <td>string</td>
        <td>
          StartedAt is when the provisioning job started<br/>
          <br/>
            <i>Format</i>: date-time<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### Domain.status.conditions[index]
<sup><sup>[↩ Parent](#domainstatus)</sup></sup>



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
