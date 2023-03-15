# Metric

This ingress controller exposes prometheus metrics on the `/metrics` endpoint. The metrics are exposed on the `:8080` port and can be scraped by prometheus or other services using typical means.

This project is built using kube-builder, so out of the box it exposes the metrics listed here

https://book.kubebuilder.io/reference/metrics-reference.html?highlight=metrics#default-exported-metrics-references

## Additional Metrics

There are no custom metrics exposed right now, but there are plans to add some in the future.
