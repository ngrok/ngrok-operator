# Custom domain

- review ingress spec for hosts (or link to ingress-to-edge-relationship.md)
- by default, you can specify any *.ngrok.io domain, and it will "just work"
- if you create a custom white label domain, the controller will attempt to set it up
- if you have your dns in place, it will just work
- if you don't, it will hang until dns is in place, the domain is fully registered/reserved, and the edge can be configured properly.
- automate that dns update with external dns (see example)
https://ngrok.com/docs/guides/how-to-set-up-a-custom-domain

