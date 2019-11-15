# live-captioning
Uses the Google Cloud Speech API to transcribe an audio stream. Deploys a set of highly available services to GKE. Work in progress.

## Setup
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
