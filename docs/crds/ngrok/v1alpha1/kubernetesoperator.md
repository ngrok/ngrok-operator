# API Reference

Packages:

- [ngrok.k8s.ngrok.com/v1alpha1](#ngrokk8sngrokcomv1alpha1)

# ngrok.k8s.ngrok.com/v1alpha1

Resource Types:

- [KubernetesOperator](#kubernetesoperator)




## KubernetesOperator
<sup><sup>[↩ Parent](#ngrokk8sngrokcomv1alpha1 )</sup></sup>






KubernetesOperator is the Schema for the ngrok kubernetesoperators API

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
      <td>KubernetesOperator</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b><a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#objectmeta-v1-meta">metadata</a></b></td>
      <td>object</td>
      <td>Refer to the Kubernetes API documentation for the fields of the `metadata` field.</td>
      <td>true</td>
      </tr><tr>
        <td><b><a href="#kubernetesoperatorspec">spec</a></b></td>
        <td>object</td>
        <td>
          <br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#kubernetesoperatorstatus">status</a></b></td>
        <td>object</td>
        <td>
          KubernetesOperatorStatus defines the observed state of KubernetesOperator<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### KubernetesOperator.spec
<sup><sup>[↩ Parent](#kubernetesoperator)</sup></sup>





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
        <td><b><a href="#kubernetesoperatorspecbinding">binding</a></b></td>
        <td>object</td>
        <td>
          Configuration for the binding feature of this Kubernetes Operator<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#kubernetesoperatorspecdeployment">deployment</a></b></td>
        <td>object</td>
        <td>
          Deployment information of this Kubernetes Operator<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>description</b></td>
        <td>string</td>
        <td>
          Description is a human-readable description of the object in the ngrok API/Dashboard<br/>
          <br/>
            <i>Default</i>: Created by ngrok-operator<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>enabledFeatures</b></td>
        <td>[]string</td>
        <td>
          Features enabled for this Kubernetes Operator<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>metadata</b></td>
        <td>string</td>
        <td>
          Metadata is a string of arbitrary data associated with the object in the ngrok API/Dashboard<br/>
          <br/>
            <i>Default</i>: {"owned-by":"ngrok-operator"}<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>region</b></td>
        <td>string</td>
        <td>
          The ngrok region in which the ingress for this operator is served. Defaults to
"global" if not specified.<br/>
          <br/>
            <i>Default</i>: global<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### KubernetesOperator.spec.binding
<sup><sup>[↩ Parent](#kubernetesoperatorspec)</sup></sup>



Configuration for the binding feature of this Kubernetes Operator

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
        <td><b>tlsSecretName</b></td>
        <td>string</td>
        <td>
          TlsSecretName is the name of the k8s secret that contains the TLS private/public keys to use for the ngrok forwarding endpoint<br/>
          <br/>
            <i>Default</i>: default-tls<br/>
        </td>
        <td>true</td>
      </tr><tr>
        <td><b>endpointSelectors</b></td>
        <td>[]string</td>
        <td>
          EndpointSelectors is a list of cel expression that determine which kubernetes-bound Endpoints will be created by the operator<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>ingressEndpoint</b></td>
        <td>string</td>
        <td>
          The public ingress endpoint for this Kubernetes Operator<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### KubernetesOperator.spec.deployment
<sup><sup>[↩ Parent](#kubernetesoperatorspec)</sup></sup>



Deployment information of this Kubernetes Operator

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
          Name is the name of the k8s deployment for the operator<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>namespace</b></td>
        <td>string</td>
        <td>
          The namespace in which the operator is deployed<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>version</b></td>
        <td>string</td>
        <td>
          The version of the operator that is currently running<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### KubernetesOperator.status
<sup><sup>[↩ Parent](#kubernetesoperator)</sup></sup>



KubernetesOperatorStatus defines the observed state of KubernetesOperator

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
        <td><b>bindingsIngressEndpoint</b></td>
        <td>string</td>
        <td>
          BindingsIngressEndpoint is the URL that the operator will use to talk
to the ngrok edge when forwarding traffic for k8s-bound endpoints<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>enabledFeatures</b></td>
        <td>string</td>
        <td>
          EnabledFeatures is the string representation of the features enabled for this Kubernetes Operator<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>errorMessage</b></td>
        <td>string</td>
        <td>
          RegistrationErrorMessage is a free-form error message if the status is error<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>id</b></td>
        <td>string</td>
        <td>
          ID is the unique identifier for this Kubernetes Operator<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>registrationErrorCode</b></td>
        <td>string</td>
        <td>
          RegistrationErrorCode is the returned ngrok error code<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>registrationStatus</b></td>
        <td>string</td>
        <td>
          RegistrationStatus is the status of the registration of this Kubernetes Operator with the ngrok API<br/>
          <br/>
            <i>Default</i>: pending<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b>uri</b></td>
        <td>string</td>
        <td>
          URI is the URI for this Kubernetes Operator<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>
