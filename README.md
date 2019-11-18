# live-captioning
Uses the Google Cloud Speech API to transcribe a live audio stream. Deploys a set of highly available services to GKE. Work in progress.

## Architecture
Not all elements of this diagram are currently implemented.
![alt text](live-captioning.png)

## Description
* The core of the app is a [regional GKE cluster](https://cloud.google.com/kubernetes-engine/docs/concepts/types-of-clusters), 
deployed over two zones
* Three independent [deployments](https://cloud.google.com/kubernetes-engine/docs/concepts/deployment) within the cluster
  * Ingest: receives the source audio and stores in Cloud Memorystore for Redis.
    * Deployed as a [Service](https://cloud.google.com/kubernetes-engine/docs/concepts/service), with a load balancer. This provides a single ingest IP address 
  * Transcribe: reads the stored audio, performs streaming recognize requests against the Cloud Speech API.
  Transcription results are stored in Redis. More details below.
  * Outputs: consume the transcription results and perform business logic. There might be several different
   flavours of Output components e.g.
     * Stream the transcriptions to a web app for review/edit
     * Persist the transcriptions for compliance/analysis
* Each deployment is replicated across the two zones for increased availability
* Cloud Memorystore for Redis is used for intermediate storage. This provides low-latency, in-memory storage
suitable for real time activities. Can be deployed in a highly-available configuration (replicated over two zones)

#### Transcribe details
* Performs [streaming recognize](https://cloud.google.com/speech-to-text/docs/basics#streaming-recognition) requests against the Cloud Speech API
* This opens a bi-directional gRPC stream to the API; audio is sent to the API, and transcription results are
asynchronously received. The Speech API client libraries abstract away the gRPC logic.
* The connection to the API is stateful; transcription results can evolve as more audio is received by the API.
* While the Transcribe deployment is replicated across zones, only a single pod (the leader) communicates with
the API at a given point
  * This is achieved using a leader election pattern.
  * The Kubernetes [Go client](https://github.com/kubernetes/client-go/tree/kubernetes-1.14.7/tools/leaderelection)
  provides some built-in logic to do leader election
  * Simply speaking, the deployed Transcribe pods compete to acquire a lock that is managed by the Kubernete control plane. 
  One pod is elected as leader for a defined period of time. The leader continually “heartbeats” to renew its position as the 
  leader, and the other pods periodically make new attempts to become the leader. This ensures that a new leader is identified quickly, 
  if the current leader fails for some reason 
  * The leader election is performed by a sidecar container, so the leader election logic is kept separate
  from the core transcription logic


## Deploy & run in GCP
* Select an existing or create a new GCP Project
* Open Cloud Shell
* Enable the relevant APIs. This can take a few mins.
  * ``gcloud services enable container.googleapis.com redis.googleapis.com cloudbuild.googleapis.com``
* Create a GKE cluster. Changes the zones to your preference. This can take a few minutes
  * ``gcloud container clusters create captioning-cluster --cluster-version=1.14.7 --region=europe-west1 --scopes=gke-default,cloud-platform --machine-type=n1-highcpu-2 --num-nodes=1 --node-locations=europe-west1-b,europe-west1-d --enable-ip-alias``
* Get the cluster credentials
  * ```gcloud container clusters get-credentials captioning-cluster --region=europe-west1```
* Create a Cloud Memorystore instance. This can taek a while
  * ```gcloud redis instances create redis-captions --tier=standard --region=europe-west1 --zone=europe-west1-b```
* Export the IP address of the Memorystore instance
  * ```export REDIS_HOST=`gcloud redis instances describe redis-captions --region=europe-west1 | sed -n 's/host: //p'` ```
* Clone this repo
  * ```git clone https://github.com/jtangney/live-captioning.git```
* Change directory
  * ```cd live-captioning```
* Build the Docker containers. They will be exported to your project's container registry
  * ```gcloud builds submit --config cloudbuild.yaml```
* Edit the yaml files to set your project ID
  * ```sed -i "s/mynewproject/$DEVSHELL_PROJECT_ID/" k8s/*.yaml```
* Edit the yaml files to set the IP address of the Cloud Memorystore instance
  * ```sed -i "s/redisHost=.*/redisHost=$REDIS_HOST/" k8s/*.yaml```
* Create the Deployments and Services in the cluster
  * ```kubectl apply -f k8s/ingestor.yaml,k8s/transcriber.yaml,k8s/editor.yaml```
* Get the external IP of the Ingest service
  * ```export INGEST_IP=`kubectl get services ingestor-service -o yaml | sed -n "s/- ip: //p"` ```
* Verify that the Ingest service is up and running
  * ```curl $INGEST_IP; echo```
