# guestcluster-quota-webhook

The guestcluster-quota-webhook is a webhook service for Rancher which does quota validations on Harvester during guest cluster creations or updates.

Normally when a quota is violated it's not noticable by the users because the quota handling is handled by Harvester in the back. So the quota vaiolation events are only visible in the Harvester clusters. In some undetected circomstances this behavior can also harm the cluster and nodes which could end up in error state.

## Covered use cases

The following use cases are covered during guest cluster creations or updates:

- CPU amount increases or decreases
- Memory amount increases or decreases
- Storage Disk sizes and amount increases or decreases
- Image Volume changes
- SSH User changes
- Machine Pool Count increases or decreases

## Not covered use cases

The following use cases can not be covered at the moment due to how Rancher has implemented the order of the object creations/updates:

- When for example poolA is upped with some resources (which should fit the quota) and poolB is upped with some resources (which exceeds the quota) then:
   - The poolA HarvesterConfig is accepted and a new VM is created (with poweroff state)
   - poolB gets denied by the webhook and stops the cluster updates
   - Then if the cluster config is within the quota and is saved again, the VM will be terminated but will stuck, this because there is no volume created. fix is to remove the finalizer in the vm

- When a NEW cluster is created through the WebUI or Terraform, the HarvesterConfigs will be created (because there is no clustername yet). Then the Cluster will be created but it's declined because it doesn't fit the quota. Result is that we have HarvesterConfig orphans.

## Extra user input checks

When testing the webhook we noticed that there is no proper input checking done in the WebUI during quest cluster creations or updates on the CPU, Memory, Storage and the Machine Pool Count. If a user configures one of those fields with a wrong (negative) value this can end up in a denial of service of the Rancher and/or Kubernetes service. This was tested against different major Rancher versions including the latest 2.9.

Therefore we added the following extra input checks for:

- CPUs
- Memory
- Image Disks
- Machine Pool Count

## kubevirt-ip-helper integration

Since v0.6.0 there is also kubevirt-ip-helper (https://github.com/joeyloman/kubevirt-ip-helper) integration which checks for available IP addresses in the Harvester backend networks. If the DHCP network is out of IP addresses the guestcluster-quota-webhook detects this and will decline the cluster creation or update request instead of fire and forget it and leave it up to the Harvester backend.

If you have multiple networks configured in Harvester and you want to free up some IP addresses by preventing new cluster creations in a specific network then you can configure some reservations in the *kubevirt-ip-helper-networks* ConfigMap. The ConfigMap should look like this:

```YAML
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubevirt-ip-helper-networks
data:
  <first_harvester_cluster_name>_<first_harvester_network_namespace>_<first_harvester_network_name>: "<ip reservation count>"
  <second_harvester_cluster_name>_<second_harvester_network_namespace>_<second_harvester_network_name>: "<ip reservation count>"
```

Where the `<\*_harvester_cluster_name>` will be for example "harvester-cluster1", the `<\*_harvester_network_namespace>` "harvester-public" and `<second_harvester_network_name>` "public-vlan-1".

If this ConfigMap doesn't exists or doesn't have any data, then the integration will be disabled.

## Prerequisites

The following components need to be installed/configured to use the guestcluster-quota-webhook:

* Rancher
* Harvester

## Building the container

There is a Dockerfile in the current directory which can be used to build the container, for example:

```SH
[docker|podman] build -t <DOCKER_REGISTRY_URI>/guestcluster-quota-webhook:latest .
```

Then push it to the remote container registry target, for example:

```SH
[docker|podman] push <DOCKER_REGISTRY_URI>/guestcluster-quota-webhook:latest
```

## Deploying the container

Use the deployment.yaml template which is located in the templates directory, for example:

```SH
kubectl create -f deployments/deployment.yaml
```

### Logging

By default only the startup, error and warning logs are enabled. More logging can be enabled by changing the LOGLEVEL environment setting in the guestcluster-quota-webhook deployment. The supported loglevels are INFO, DEBUG and TRACE.

### Operate mode

By default the webhook denies all Quota violations. This means the guest cluster object changes are declined and you will get a webhook error message. So nothing happens with the cluster. If you only want to log these violations and still permit the guest cluster object updates you can set the OPERATEMODE environment setting to LOGONLY in the guestcluster-quota-webhook deployment.

### Default namespace deployment

By default the webhook will be deployed in the questcluster-quota-webbook namespace. If you want to deploy it in another namespace you can change the namespace references in the deployment.yaml and you need to set the KUBENAMESPACE environment variable of the application.

### Metrics

The following metrics are included in the application which can be used for monitoring:

```YAML
Name: questclusterquotawebhook_app_logs
Description: Amount of warnings or errors detected.
```

```YAML
Name: guestclusterquotawebhook_app_actions
Description: Amount of accepted or blocked actions per resource type.
```

# License

Copyright (c) 2025 Joey Loman <joey@binbash.org>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

[http://www.apache.org/licenses/LICENSE-2.0](http://www.apache.org/licenses/LICENSE-2.0)

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
