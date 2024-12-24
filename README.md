# guestcluster-quota-webhook

The guestcluster-quota-webhook is a webhook service for Rancher which does Quota validations on Harvester during guest cluster creations or updates.

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

# License

Copyright (c) 2024 Joey Loman <joey@binbash.org>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

[http://www.apache.org/licenses/LICENSE-2.0](http://www.apache.org/licenses/LICENSE-2.0)

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
