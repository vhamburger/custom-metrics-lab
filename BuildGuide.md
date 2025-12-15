# Build Guide: 
Building and Pushing the Worker ImageThese instructions will guide you through setting up Google Artifact Registry, building the worker Go application into a Docker container, and pushing it to the registry so GKE can access it.Run these commands from your Cloud Shell or a local terminal where gcloud and docker are installed.

### 1. Prerequisites: 
* Set Environment VariablesFirst, set up variables to make the commands easier to run.

#### 1. Set your Google Cloud Project ID

    export PROJECT_ID=$(gcloud config get-value project)

#### 2. Set the region for your repository (e.g., us-central1)

    export REGION="us-central1"

#### 3. Set a name for your Artifact Registry repository

    export REPO_NAME="autoscale-lab-repo"

#### 4. Set the name and tag for your worker image

    export IMAGE_NAME="autoscale-worker"
    export IMAGE_TAG="v1.0.0"

#### 5. This variable creates the full image path

    export IMAGE_URI="${REGION}-docker.pkg.dev/${PROJECT_ID}/${REPO_NAME}/${IMAGE_NAME}:${IMAGE_TAG}"

#### 6. Verify your variables

    echo "Project ID: ${PROJECT_ID}"
    echo "Image URI: ${IMAGE_URI}"

### 2. Enable Services and Create RepositoryWe only need to do this once per project.# 1. Enable the Artifact Registry API
gcloud services enable artifactregistry.googleapis.com

#### 1. Create the Docker repository in Artifact Registry
(This command will safely error if the repo already exists)

    gcloud artifacts repositories create "${REPO_NAME}" \
        --repository-format=docker \
        --location="${REGION}" \
        --description="Repository for autoscale lab images"

#### 2. Configure Docker to authenticate with Artifact Registry

    gcloud auth configure-docker "${REGION}-docker.pkg.dev"

### 3. Build and Push the Docker Image
Now, we'll build the image using the provided Dockerfile.# 1. Navigate to the 'worker' directory (where the Dockerfile is)
(Assuming you are in the root of the project)
cd worker

#### 1. Build the Docker image
The '-t' tags the image with the URI we created.
The '.' tells Docker to build from the current directory.

    docker build -t "${IMAGE_URI}" .

#### 2. Push the image to Artifact Registry

    docker push "${IMAGE_URI}"

#### 3. Verification
Confirm your image is uploaded and ready for GKE.# List images in the repository

    gcloud artifacts docker images list "${REGION}-docker.pkg.dev/${PROJECT_ID}/${REPO_NAME}"

You should see your autoscale-worker image with the tag v1.0.0. You can now use the IMAGE_URI variable in the main lab guide.
