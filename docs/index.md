---
hide:
- navigation
- toc
---

Welcome.

## Plan

To create a Webhook relay system for GitHub webhooks from outside to inside a protected environment - not accesible from the Internet.

### Server

Receive and cache webhooks events from GitHub

  * only respond to configured repositories
  * only "relay" to registered, authenticated, and authorized clients
 
### Client

Connect to server and receive the webhook events and send them to a target URL

  * has an identity
  * connects to server with GRPC
  * only relays received events to healthy endpoints
    * needs a health / loadbalance mechanism

## Packaging

* docker image
* carvel image bundle
* kapp deployment
  * package
  * package repository
* helm chart (optional)