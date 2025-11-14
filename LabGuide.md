# **Lab Guide: GKE Autoscaling with Custom Metrics (Pub/Sub)**

**Objective:** This lab demonstrates why CPU-based autoscaling is sometimes insufficient and how to autoscale a GKE workload based on a custom metric (the number of jobs in a Pub/Sub queue) instead.

Scenario:  
We have a worker that pulls jobs from a Pub/Sub queue. Each job takes 90 seconds to complete but consumes very little CPU. A CPU-based HPA will fail to scale, leading to a long backlog. An HPA that reads our custom metric, autoscale\_lab\_jobs\_in\_queue, will scale correctly and process the queue quickly.

## **Step 0: Set Environment Variables**

Open Cloud Shell and run these commands to configure your environment.

\# 1\. Your Project ID  
export PROJECT\_ID=$(gcloud config get-value project)

\# 2\. Zone/Region for the GKE cluster  
export CLUSTER\_LOCATION="us-central1"  
export CLUSTER\_ZONE="us-central1-c"

\# 3\. GKE cluster name  
export CLUSTER\_NAME="autoscale-lab-cluster"

\# 4\. Pub/Sub resources  
export TOPIC\_ID="autoscale-lab-topic"  
export SUB\_ID="autoscale-lab-sub"

\# 5\. IAM / K8s service accounts  
export K8S\_NS="autoscale-lab"  
export K8S\_SA="worker-ksa"  
export GSA\_NAME="pubsub-worker-gsa"  
export GSA\_EMAIL="${GSA\_NAME}@${PROJECT\_ID}.iam.gserviceaccount.com"

\# 6\. Your worker image URI (from the build step, you should have run building\_the\_worker.md)  
\#    \!\! PLEASE REPLACE THIS VALUE \!\!  
export IMAGE\_URI="us-central1-docker.pkg.dev/${PROJECT\_ID}/autoscale-lab-repo/autoscale-worker:v1.0.0"

\# 7\. Verify your variables  
echo "Project ID: ${PROJECT\_ID}"  
echo "Image URI: ${IMAGE\_URI}"  
\# (Ensure the output is correct before proceeding)

## **Step 1: Create GKE Cluster and Pub/Sub**

We'll create a GKE cluster with **Workload Identity** enabled and our Pub/Sub resources.

\# 1\. Create GKE cluster with Workload Identity enabled  
gcloud container clusters create "${CLUSTER\_NAME}" \\  
    \--zone "${CLUSTER\_ZONE}" \\  
    \--workload-pool="${PROJECT\_ID}.svc.id.goog" \\  
    \--enable-managed-prometheus \\  
    \--scopes="\[https://www.googleapis.com/auth/cloud-platform\](https://www.googleapis.com/auth/cloud-platform)"

\# 2\. Get cluster credentials  
gcloud container clusters get-credentials "${CLUSTER\_NAME}" \--zone "${CLUSTER\_ZONE}"

\# 3\. Create Pub/Sub topic  
gcloud pubsub topics create "${TOPIC\_ID}"

\# 4\. Create Pub/Sub subscription  
\#    IMPORTANT: Set \--ack-deadline to 180s (3 min), longer than our 90s job  
gcloud pubsub subscriptions create "${SUB\_ID}" \\  
    \--topic="${TOPIC\_ID}" \\  
    \--ack-deadline=180

## **Step 2: Configure Workload Identity (IAM)**

We'll grant our GKE pod permission to read Pub/Sub messages.

\# 1\. Create the Google Service Account (GSA)  
gcloud iam service-accounts create "${GSA\_NAME}" \\  
    \--display-name="Autoscale Lab Pub/Sub Worker"

\# 2\. Grant the GSA permission to read from the subscription  
gcloud pubsub subscriptions add-iam-policy-binding "${SUB\_ID}" \\  
    \--member="serviceAccount:${GSA\_EMAIL}" \\  
    \--role="roles/pubsub.subscriber"

\# 3\. Create the Kubernetes Namespace (K8S\_NS)  
kubectl create namespace "${K8S\_NS}"

\# 4\. Create the Kubernetes Service Account (KSA)  
kubectl create serviceaccount "${K8S\_SA}" \--namespace "${K8S\_NS}"

\# 5\. Bind the GSA and KSA (Workload Identity)  
gcloud iam service-accounts add-iam-policy-binding "${GSA\_EMAIL}" \\  
    \--role="roles/iam.workloadIdentityUser" \\  
    \--member="serviceAccount:${PROJECT\_ID}.svc.id.goog\[${K8S\_NS}/${K8S\_SA}\]"

\# 6\. Annotate the KSA (so GKE knows the link)  
kubectl annotate serviceaccount "${K8S\_SA}" \\  
    \--namespace "${K8S\_NS}" \\  
    iam.gke.io/gcp-service-account="${GSA\_EMAIL}"

## **Step 3: Deploy the Worker**

Now we deploy our worker pod using the manifests in kubernetes/gke\_manifests.yaml.

1. **Edit kubernetes/gke\_manifests.yaml** to replace YOUR\_PROJECT\_ID (2 occurrences) and the image: placeholder with your values.

\# TIP: Use sed if you're executing this from the repo root  
sed \-i "s|YOUR\_PROJECT\_ID|${PROJECT\_ID}|g" kubernetes/gke\_manifests.yaml  
sed \-i "s|us-central1-docker.pkg.dev/YOUR\_PROJECT\_ID/autoscale-lab-repo/autoscale-worker:v1.0.0|${IMAGE\_URI}|g" kubernetes/gke\_manifests.yaml

\# 3\. Apply the manifests  
kubectl apply \-f kubernetes/gke\_manifests.yaml

**Verification:** Wait for the pod to be running.

kubectl get pods \--namespace "${K8S\_NS}" \-w  
\# (Wait until the status is "Running")

## **Step 4: "Before" Test (CPU Scaling)**

We'll prove that CPU scaling is unsuitable.

\# 1\. Apply the CPU HPA (hpa-cpu.yaml)  
kubectl apply \-f kubernetes/hpa-cpu.yaml

\# 2\. Open a SECOND Cloud Shell tab  
\#    Watch the HPA:  
kubectl get hpa \--namespace "${K8S\_NS}" \-w

\# 3\. Open a THIRD Cloud Shell tab  
\#    Watch the Pods:  
kubectl get pods \--namespace "${K8S\_NS}" \-w

\# 4\. Open a FOURTH Cloud Shell tab  
\#    Watch the worker's logs:  
kubectl logs \-f \-n "${K8S\_NS}" \-l app=worker

\# 5\. In your FIRST tab, start the publisher in "auto" mode  
\#    (cd into the 'publisher' directory first)  
go run . auto "${PROJECT\_ID}" "${TOPIC\_ID}" "${SUB\_ID}"

**What you will observe:**

* **HPA (Tab 2):** The HPA will stay at low CPU utilization (e.g., 10%/50%). No scaling is triggered.  
* **Pods (Tab 3):** **No new pods** will be created. The single pod processes all 34 jobs sequentially (takes 10+ minutes).

**Clean up:**

\# Stop the 'watch' commands (Ctrl+C in Tabs 2, 3, 4\)  
kubectl delete \-f kubernetes/hpa-cpu.yaml

## **Step 5: "After" Test (Custom Metric)**

Now we'll install the Prometheus adapter and use our metric-based HPA.

\# 1\. Install the in-cluster Prometheus Adapter  
\#    This allows the HPA to use 'type: Pods' for Prometheus metrics  
kubectl apply \-f \[https://raw.githubusercontent.com/GoogleCloudPlatform/k8s-prometheus-adapter/master/adapter.yaml\](https://raw.githubusercontent.com/GoogleCloudPlatform/k8s-prometheus-adapter/master/adapter.yaml)

\# (Wait \~1 minute for the adapter to start)  
kubectl get pods \-n custom-metrics

\# 2\. Apply the metric HPA (hpa.yaml)  
kubectl apply \-f kubernetes/hpa.yaml

\# 3\. Set up your observation tabs just like before:  
\#    Tab 2: Watch the HPA (will now show "autoscale\_lab\_jobs\_in\_queue")  
kubectl get hpa \--namespace "${K8S\_NS}" \-w

\# 4\. Purge the queue (to ensure a clean start)  
\#    (In the 'publisher' directory in your FIRST tab)  
go run . purge "${PROJECT\_ID}" "${TOPIC\_ID}" "${SUB\_ID}"

**Start the Test\!**

\# 5\. In your FIRST tab, start the publisher in "auto" mode  
go run . auto "${PROJECT\_ID}" "${TOPIC\_ID}" "${SUB\_ID}"

**What you will observe:**

* **HPA (Tab 2):** The HPA will see the target is **9/1** (Current jobs: 9, Target: 1 per pod). It will immediately calculate **9 replicas** and start scaling.  
* **Pods (Tab 3):** You will see 8 new pods spin up, and the job processing time will be drastically reduced (minutes instead of 10+ minutes).

## **Step 6: Clean Up**

\# 1\. Delete the HPA, Deployment, and Adapter resources  
kubectl delete \-f kubernetes/hpa.yaml  
kubectl delete \-f kubernetes/gke\_manifests.yaml  
kubectl delete \-f \[https://raw.githubusercontent.com/GoogleCloudPlatform/k8s-prometheus-adapter/master/adapter.yaml\](https://raw.githubusercontent.com/GoogleCloudPlatform/k8s-prometheus-adapter/master/adapter.yaml)  
kubectl delete namespace "${K8S\_NS}"

\# 2\. Delete the IAM bindings and service accounts  
gcloud pubsub subscriptions remove-iam-policy-binding "${SUB\_ID}" \\  
    \--member="serviceAccount:${GSA\_EMAIL}" \\  
    \--role="roles/pubsub.subscriber"

gcloud iam service-accounts delete "${GSA\_EMAIL}" \--quiet

\# 3\. Delete the Pub/Sub resources  
gcloud pubsub subscriptions delete "${SUB\_ID}"  
gcloud pubsub topics delete "${TOPIC\_ID}"

\# 4\. Delete the GKE cluster  
gcloud container clusters delete "${CLUSTER\_NAME}" \--zone "${CLUSTER\_ZONE}" \--quiet  
