# API Reference

Packages:

- [ngrok.k8s.ngrok.com/v1alpha1](#ngrokk8sngrokcomv1alpha1)

# ngrok.k8s.ngrok.com/v1alpha1

Resource Types:

- [NgrokTrafficPolicy](#ngroktrafficpolicy)




## NgrokTrafficPolicy
<sup><sup>[↩ Parent](#ngrokk8sngrokcomv1alpha1 )</sup></sup>






NgrokTrafficPolicy is the Schema for the ngroktrafficpolicies API

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
      <td>NgrokTrafficPolicy</td>
      <td>true</td>
      </tr>
      <tr>
      <td><b><a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#objectmeta-v1-meta">metadata</a></b></td>
      <td>object</td>
      <td>Refer to the Kubernetes API documentation for the fields of the `metadata` field.</td>
      <td>true</td>
      </tr><tr>
        <td><b><a href="#ngroktrafficpolicyspec">spec</a></b></td>
        <td>object</td>
        <td>
          NgrokTrafficPolicySpec defines the desired state of NgrokTrafficPolicy<br/>
        </td>
        <td>false</td>
      </tr><tr>
        <td><b><a href="#ngroktrafficpolicystatus">status</a></b></td>
        <td>object</td>
        <td>
          NgrokTrafficPolicyStatus defines the observed state of NgrokTrafficPolicy<br/>
        </td>
        <td>false</td>
      </tr></tbody>
</table>


### NgrokTrafficPolicy.spec
<sup><sup>[↩ Parent](#ngroktrafficpolicy)</sup></sup>



NgrokTrafficPolicySpec defines the desired state of NgrokTrafficPolicy

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


### NgrokTrafficPolicy.status
<sup><sup>[↩ Parent](#ngroktrafficpolicy)</sup></sup>



NgrokTrafficPolicyStatus defines the observed state of NgrokTrafficPolicy

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
