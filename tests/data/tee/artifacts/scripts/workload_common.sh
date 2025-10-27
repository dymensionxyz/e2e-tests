#!/bin/bash
#
# Common functions for TEE Fullnode deployment

set_gcp_project() {
    local project_id=$1
    echo "Setting GCP project to ${project_id}..."
    gcloud config set project "${project_id}"
}

create_service_account() {
    local sa_name=$1
    if gcloud iam service-accounts describe "${sa_name}@${PROJECT_ID}.iam.gserviceaccount.com" \
        --project="${PROJECT_ID}" &>/dev/null; then
        echo "Service account ${sa_name} already exists"
    else
        echo "Creating service account ${sa_name}..."
        gcloud iam service-accounts create "${sa_name}" \
            --display-name="${sa_name}" \
            --project="${PROJECT_ID}"
    fi
}

grant_workload_user_role_to_service_account() {
    local sa_name=$1
    local project_id=$2
    echo "Granting confidentialcomputing.workloadUser role to ${sa_name}..."
    gcloud projects add-iam-policy-binding "${project_id}" \
        --member="serviceAccount:${sa_name}@${project_id}.iam.gserviceaccount.com" \
        --role="roles/confidentialcomputing.workloadUser" \
        --quiet
}

grant_log_writer_role_to_service_account() {
    local sa_name=$1
    local project_id=$2
    echo "Granting logging.logWriter role to ${sa_name}..."
    gcloud projects add-iam-policy-binding "${project_id}" \
        --member="serviceAccount:${sa_name}@${project_id}.iam.gserviceaccount.com" \
        --role="roles/logging.logWriter" \
        --quiet
}

create_artifact_repository() {
    local repo_name=$1
    local location=$2
    if gcloud artifacts repositories describe "${repo_name}" \
        --location="${location}" \
        --project="${PROJECT_ID}" &>/dev/null; then
        echo "Artifact repository ${repo_name} already exists"
    else
        echo "Creating artifact repository ${repo_name}..."
        gcloud artifacts repositories create "${repo_name}" \
            --repository-format=docker \
            --location="${location}" \
            --project="${PROJECT_ID}"
    fi
}

delete_instance_if_exists() {
    local instance_name=$1
    local zone=$2
    if gcloud compute instances describe "${instance_name}" \
        --zone="${zone}" \
        --project="${PROJECT_ID}" &>/dev/null; then
        echo "Deleting existing instance ${instance_name}..."
        gcloud compute instances delete "${instance_name}" \
            --zone="${zone}" \
            --project="${PROJECT_ID}" \
            --quiet
    fi
}

create_firewall_rule() {
    local rule_name=$1
    local port=$2
    local sa_email="${WORKLOAD_SERVICE_ACCOUNT}@${PROJECT_ID}.iam.gserviceaccount.com"
    
    if gcloud compute firewall-rules describe "${rule_name}" \
        --project="${PROJECT_ID}" &>/dev/null; then
        echo "Firewall rule ${rule_name} already exists"
    else
        echo "Creating firewall rule ${rule_name} for port ${port}..."
        gcloud compute firewall-rules create "${rule_name}" \
            --allow "tcp:${port}" \
            --source-ranges 0.0.0.0/0 \
            --target-service-accounts "${sa_email}" \
            --project="${PROJECT_ID}"
    fi
}

wait_for_instance() {
    local instance_name=$1
    local zone=$2
    local max_attempts=30
    local attempt=0
    
    echo "Waiting for instance ${instance_name} to be ready..."
    while [ $attempt -lt $max_attempts ]; do
        if gcloud compute instances describe "${instance_name}" \
            --zone="${zone}" \
            --project="${PROJECT_ID}" \
            --format="value(status)" | grep -q "RUNNING"; then
            echo "Instance ${instance_name} is running"
            return 0
        fi
        echo "Waiting... (attempt $((attempt+1))/${max_attempts})"
        sleep 10
        attempt=$((attempt+1))
    done
    
    echo "ERROR: Instance ${instance_name} failed to start"
    return 1
}

get_instance_external_ip() {
    local instance_name=$1
    local zone=$2
    gcloud compute instances describe "${instance_name}" \
        --zone="${zone}" \
        --format="value(networkInterfaces[0].accessConfigs[0].natIP)" \
        --project="${PROJECT_ID}"
}