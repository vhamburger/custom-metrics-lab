# GKE Autoscaling with Custom Metrics Lab

This repository contains the source code and configuration files for a lab demonstrating the necessity of custom metrics over traditional CPU utilization for autoscaling message queue workloads in Google Kubernetes Engine (GKE).

## Scenario

A Go worker processes messages from a Pub/Sub queue. Each job requires 90 seconds of wall time but generates low CPU load. Scaling based on CPU fails to address the queue backlog, while scaling based on the autoscale_lab_jobs_in_queue custom metric (which reports the queue size) correctly scales the worker pods to meet demand.

## Prerequisites

1. A Google Cloud Project with Billing Enabled.
2. Cloud Shell or a local environment with gcloud, kubectl, and docker installed.
3. Go development environment (to run the publisher utility).
4. The Docker image for the worker must be built and pushed to Artifact Registry (using the instructions in building_the_worker.md).

## Getting Started

1. Build and Push the Worker: Follow the instructions in building_the_worker.md to containerize the application.
2. Start the Lab: Open docs/lab_guide.md and follow the steps from Step 0 onwards to set up GKE, IAM, and run the comparison test.

## Cleanup
Follow Step 6 in docs/lab_guide.md to destroy all cloud resources.
