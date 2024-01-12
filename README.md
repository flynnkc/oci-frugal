# OCI Frugal

The purpose of this software is to run a program that can start and stop paid OCI services on a schedule. Tuning resources to run only when needed can lower costs associated with running cloud services. Using the lightweight threading tools provided by the Go language, scaling will be done in an efficient, timely manner.

## Goals

The goals I am setting out with for this package include:

- [ ] Custom schedule selection via tagging on OCI resources
- [ ] Logic to create and update tag namespaces
- [ ] Setting schedule tags to required
- [ ] Custom schedule definitions via YAML file input
- [ ] Container task multiplexing

## Current Task

__Logic to create and update tag namespaces__
