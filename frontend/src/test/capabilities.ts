import type { Capability } from '../features/platform/capabilities';

// A recorded response of GET /resource-definition-capabilities from the product
// backend over its packaged, verified module registry (secret-free: pins,
// variables, outputs and provider requirements only; the loader fills the
// omitted empty arrays). Only verified registry modules are registrable
// (ADR-014); the static `kubernetes` pairs are no longer published.
export const capabilityFixture: Capability[] = [
 {
  "resourceType": "k8s-cluster",
  "driverType": "terraform",
  "module": "eks",
  "source": {
   "sourceId": "platform",
   "moduleId": "eks",
   "revision": "v3-2026.10.13",
   "subdirectory": "eks",
   "contentSha256": "15cd6a4d5a88dc48ff49cce5c84c68db160f934d8594752b933b502952647997"
  },
  "terraformVersion": "1.9.8",
  "providerRequirements": [
   {
    "name": "aws",
    "source": "registry.terraform.io/hashicorp/aws",
    "version": "6.60.0",
    "binding": "aws"
   }
  ],
  "outputs": [
   {
    "name": "cluster_security_group_id",
    "sensitive": false
   },
   {
    "name": "endpoint",
    "sensitive": false
   },
   {
    "name": "name",
    "sensitive": false
   },
   {
    "name": "node_group_name",
    "sensitive": false
   },
   {
    "name": "node_role_arn",
    "sensitive": false
   },
   {
    "name": "kubeContext",
    "sensitive": false
   },
   {
    "name": "kubeconfig",
    "sensitive": false
   }
  ],
  "variables": [
   {
    "name": "ebs_csi_driver",
    "type": "bool",
    "required": false,
    "hasDefault": true,
    "default": false,
    "resourceParam": false
   },
   {
    "name": "kubernetes_version",
    "type": "string",
    "required": true,
    "hasDefault": false,
    "resourceParam": false
   },
   {
    "name": "name",
    "type": "string",
    "required": true,
    "hasDefault": false,
    "resourceParam": false,
    "preset": "${context.infra.resourceName}"
   },
   {
    "name": "node_ami_type",
    "type": "string",
    "required": true,
    "hasDefault": false,
    "resourceParam": false
   },
   {
    "name": "node_capacity_type",
    "type": "string",
    "required": true,
    "hasDefault": false,
    "resourceParam": false
   },
   {
    "name": "node_disk_size",
    "type": "number",
    "required": true,
    "hasDefault": false,
    "resourceParam": false
   },
   {
    "name": "node_instance_type",
    "type": "string",
    "required": true,
    "hasDefault": false,
    "resourceParam": false
   },
   {
    "name": "public_access_cidrs",
    "type": "list(string)",
    "required": true,
    "hasDefault": false,
    "resourceParam": false
   },
   {
    "name": "region",
    "type": "string",
    "required": true,
    "hasDefault": false,
    "resourceParam": false
   },
   {
    "name": "subnet_ids",
    "type": "list(string)",
    "required": true,
    "hasDefault": false,
    "resourceParam": false,
    "preset": "${resources['vpc.default#@infra'].outputs.subnetIds}"
   }
  ],
  "executorOwned": [
   "tags"
  ],
  "account": {
   "required": true,
   "type": "AWS",
   "regionVariable": "region"
  },
  "bindings": [
   {
    "provider": "aws",
    "kind": "aws",
    "targets": [
     {
      "target": "account"
     }
    ]
   }
  ]
 },
 {
  "resourceType": "k8s-namespace",
  "driverType": "terraform",
  "module": "k8s-namespace",
  "source": {
   "sourceId": "platform",
   "moduleId": "k8s-namespace",
   "revision": "v3-2026.10.13",
   "subdirectory": "k8s-namespace",
   "contentSha256": "836f7b7470deb519633a523dccb02a83a1b20947725ddb705596fb2601a180a8"
  },
  "terraformVersion": "1.9.8",
  "providerRequirements": [
   {
    "name": "kubernetes",
    "source": "registry.terraform.io/hashicorp/kubernetes",
    "version": "2.38.0",
    "binding": "kubernetes"
   }
  ],
  "outputs": [
   {
    "name": "name",
    "sensitive": false
   }
  ],
  "variables": [
   {
    "name": "name",
    "type": "string",
    "required": true,
    "hasDefault": false,
    "resourceParam": false,
    "preset": "${context.env.namespace}"
   }
  ],
  "executorOwned": [
   "tags"
  ],
  "account": {
   "required": false
  },
  "bindings": [
   {
    "provider": "kubernetes",
    "kind": "kubernetes",
    "targets": [
     {
      "target": "resource"
     },
     {
      "target": "environment",
      "legacy": true
     }
    ]
   }
  ]
 },
 {
  "resourceType": "postgres",
  "driverType": "terraform",
  "module": "aurora",
  "source": {
   "sourceId": "platform",
   "moduleId": "aurora",
   "revision": "v3-2026.10.13",
   "subdirectory": "aurora",
   "contentSha256": "f8755b77289c2e2bf5b2be351b90d4a1d44d0eeaf56021560c7fefb1aafc4ba6"
  },
  "terraformVersion": "1.9.8",
  "providerRequirements": [
   {
    "name": "aws",
    "source": "registry.terraform.io/hashicorp/aws",
    "version": "6.60.0",
    "binding": "aws"
   }
  ],
  "outputs": [
   {
    "name": "cluster_identifier",
    "sensitive": false
   },
   {
    "name": "database",
    "sensitive": false
   },
   {
    "name": "host",
    "sensitive": false
   },
   {
    "name": "password",
    "sensitive": true
   },
   {
    "name": "port",
    "sensitive": false
   },
   {
    "name": "security_group_id",
    "sensitive": false
   },
   {
    "name": "username",
    "sensitive": false
   },
   {
    "name": "writer_identifier",
    "sensitive": false
   }
  ],
  "variables": [
   {
    "name": "database",
    "type": "string",
    "required": true,
    "hasDefault": false,
    "resourceParam": true
   },
   {
    "name": "engine_version",
    "type": "string",
    "required": true,
    "hasDefault": false,
    "resourceParam": false
   },
   {
    "name": "max_capacity",
    "type": "number",
    "required": true,
    "hasDefault": false,
    "resourceParam": false
   },
   {
    "name": "min_capacity",
    "type": "number",
    "required": true,
    "hasDefault": false,
    "resourceParam": false
   },
   {
    "name": "name",
    "type": "string",
    "required": true,
    "hasDefault": false,
    "resourceParam": false,
    "preset": "${context.infra.resourceName}"
   },
   {
    "name": "private_subnet_ids",
    "type": "list(string)",
    "required": true,
    "hasDefault": false,
    "resourceParam": false,
    "preset": "${resources['vpc.default#@infra'].outputs.privateSubnetIds}"
   },
   {
    "name": "region",
    "type": "string",
    "required": true,
    "hasDefault": false,
    "resourceParam": false
   },
   {
    "name": "seconds_until_auto_pause",
    "type": "number",
    "required": false,
    "hasDefault": true,
    "default": 3600,
    "resourceParam": false
   },
   {
    "name": "subnet_ids",
    "type": "list(string)",
    "required": true,
    "hasDefault": false,
    "resourceParam": false,
    "preset": "${resources['vpc.default#@infra'].outputs.subnetIds}"
   },
   {
    "name": "username",
    "type": "string",
    "required": true,
    "hasDefault": false,
    "resourceParam": true
   },
   {
    "name": "vpc_id",
    "type": "string",
    "required": true,
    "hasDefault": false,
    "resourceParam": false,
    "preset": "${resources['vpc.default#@infra'].outputs.id}"
   }
  ],
  "executorOwned": [
   "master_password",
   "tags"
  ],
  "account": {
   "required": true,
   "type": "AWS",
   "regionVariable": "region"
  },
  "bindings": [
   {
    "provider": "aws",
    "kind": "aws",
    "targets": [
     {
      "target": "account"
     }
    ]
   }
  ]
 },
 {
  "resourceType": "postgres",
  "driverType": "terraform",
  "module": "postgres-k8s",
  "source": {
   "sourceId": "platform",
   "moduleId": "postgres-k8s",
   "revision": "v3-2026.10.13",
   "subdirectory": "postgres-k8s",
   "contentSha256": "446aeb69eafa37573233ff53629372172b8ce80439e8c8c2440fdbede9a860e1"
  },
  "terraformVersion": "1.9.8",
  "providerRequirements": [
   {
    "name": "kubernetes",
    "source": "registry.terraform.io/hashicorp/kubernetes",
    "version": "2.38.0",
    "binding": "kubernetes"
   }
  ],
  "outputs": [
   {
    "name": "database",
    "sensitive": false
   },
   {
    "name": "host",
    "sensitive": false
   },
   {
    "name": "password",
    "sensitive": true
   },
   {
    "name": "port",
    "type": "number",
    "sensitive": false
   },
   {
    "name": "username",
    "sensitive": false
   }
  ],
  "variables": [
   {
    "name": "annotations",
    "type": "map(string)",
    "required": false,
    "hasDefault": true,
    "default": {},
    "resourceParam": false
   },
   {
    "name": "database",
    "type": "string",
    "required": true,
    "hasDefault": false,
    "resourceParam": true
   },
   {
    "name": "image",
    "type": "string",
    "required": false,
    "hasDefault": true,
    "default": "postgres:16.14-alpine@sha256:57c72fd2a128e416c7fcc499958864df5301e940bca0a56f58fddf30ffc07777",
    "resourceParam": false
   },
   {
    "name": "labels",
    "type": "map(string)",
    "required": false,
    "hasDefault": true,
    "default": {},
    "resourceParam": false
   },
   {
    "name": "namespace",
    "type": "string",
    "required": true,
    "hasDefault": false,
    "resourceParam": false,
    "preset": "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}"
   },
   {
    "name": "storage",
    "type": "string",
    "required": false,
    "hasDefault": true,
    "default": "1Gi",
    "resourceParam": false
   },
   {
    "name": "storage_class",
    "type": "string",
    "required": false,
    "hasDefault": true,
    "default": "",
    "resourceParam": false
   },
   {
    "name": "username",
    "type": "string",
    "required": true,
    "hasDefault": false,
    "resourceParam": true
   }
  ],
  "executorOwned": [
   "password",
   "tags"
  ],
  "account": {
   "required": false
  },
  "bindings": [
   {
    "provider": "kubernetes",
    "kind": "kubernetes",
    "targets": [
     {
      "target": "resource"
     },
     {
      "target": "environment",
      "legacy": true
     }
    ]
   }
  ]
 },
 {
  "resourceType": "redis",
  "driverType": "terraform",
  "module": "elasticache",
  "source": {
   "sourceId": "platform",
   "moduleId": "elasticache",
   "revision": "v3-2026.10.13",
   "subdirectory": "elasticache",
   "contentSha256": "b46261d722e23f7ace3693c12f0d5ed03f28ca6d8abbbc8ae519230051c3e8e7"
  },
  "terraformVersion": "1.9.8",
  "providerRequirements": [
   {
    "name": "aws",
    "source": "registry.terraform.io/hashicorp/aws",
    "version": "6.60.0",
    "binding": "aws"
   }
  ],
  "outputs": [
   {
    "name": "host",
    "sensitive": false
   },
   {
    "name": "password",
    "sensitive": true
   },
   {
    "name": "port",
    "sensitive": false
   },
   {
    "name": "replication_group_id",
    "sensitive": false
   },
   {
    "name": "security_group_id",
    "sensitive": false
   },
   {
    "name": "tls",
    "sensitive": false
   },
   {
    "name": "username",
    "sensitive": false
   }
  ],
  "variables": [
   {
    "name": "engine_version",
    "type": "string",
    "required": true,
    "hasDefault": false,
    "resourceParam": false
   },
   {
    "name": "node_type",
    "type": "string",
    "required": true,
    "hasDefault": false,
    "resourceParam": false
   },
   {
    "name": "private_subnet_ids",
    "type": "list(string)",
    "required": true,
    "hasDefault": false,
    "resourceParam": false,
    "preset": "${resources['vpc.default#@infra'].outputs.privateSubnetIds}"
   },
   {
    "name": "region",
    "type": "string",
    "required": true,
    "hasDefault": false,
    "resourceParam": false
   },
   {
    "name": "subnet_ids",
    "type": "list(string)",
    "required": true,
    "hasDefault": false,
    "resourceParam": false,
    "preset": "${resources['vpc.default#@infra'].outputs.subnetIds}"
   },
   {
    "name": "username",
    "type": "string",
    "required": false,
    "hasDefault": true,
    "default": "app",
    "resourceParam": false
   },
   {
    "name": "vpc_id",
    "type": "string",
    "required": true,
    "hasDefault": false,
    "resourceParam": false,
    "preset": "${resources['vpc.default#@infra'].outputs.id}"
   }
  ],
  "executorOwned": [
   "password",
   "tags"
  ],
  "account": {
   "required": true,
   "type": "AWS",
   "regionVariable": "region"
  },
  "bindings": [
   {
    "provider": "aws",
    "kind": "aws",
    "targets": [
     {
      "target": "account"
     }
    ]
   }
  ]
 },
 {
  "resourceType": "redis",
  "driverType": "terraform",
  "module": "redis-k8s",
  "source": {
   "sourceId": "platform",
   "moduleId": "redis-k8s",
   "revision": "v3-2026.10.13",
   "subdirectory": "redis-k8s",
   "contentSha256": "feaa167e8a79990aa57e5a6f5f5c5a3de15c2c82312bffe179557e25e1d14e74"
  },
  "terraformVersion": "1.9.8",
  "providerRequirements": [
   {
    "name": "kubernetes",
    "source": "registry.terraform.io/hashicorp/kubernetes",
    "version": "2.38.0",
    "binding": "kubernetes"
   }
  ],
  "outputs": [
   {
    "name": "host",
    "sensitive": false
   },
   {
    "name": "password",
    "sensitive": true
   },
   {
    "name": "port",
    "type": "number",
    "sensitive": false
   },
   {
    "name": "tls",
    "type": "bool",
    "sensitive": false
   },
   {
    "name": "username",
    "sensitive": false
   }
  ],
  "variables": [
   {
    "name": "annotations",
    "type": "map(string)",
    "required": false,
    "hasDefault": true,
    "default": {},
    "resourceParam": false
   },
   {
    "name": "image",
    "type": "string",
    "required": false,
    "hasDefault": true,
    "default": "redis:7.4.7-alpine@sha256:02f2cc4882f8bf87c79a220ac958f58c700bdec0dfb9b9ea61b62fb0e8f1bfcf",
    "resourceParam": false
   },
   {
    "name": "labels",
    "type": "map(string)",
    "required": false,
    "hasDefault": true,
    "default": {},
    "resourceParam": false
   },
   {
    "name": "namespace",
    "type": "string",
    "required": true,
    "hasDefault": false,
    "resourceParam": false,
    "preset": "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}"
   },
   {
    "name": "storage",
    "type": "string",
    "required": false,
    "hasDefault": true,
    "default": "1Gi",
    "resourceParam": false
   },
   {
    "name": "storage_class",
    "type": "string",
    "required": false,
    "hasDefault": true,
    "default": "",
    "resourceParam": false
   },
   {
    "name": "username",
    "type": "string",
    "required": false,
    "hasDefault": true,
    "default": "app",
    "resourceParam": false
   }
  ],
  "executorOwned": [
   "password",
   "tags"
  ],
  "account": {
   "required": false
  },
  "bindings": [
   {
    "provider": "kubernetes",
    "kind": "kubernetes",
    "targets": [
     {
      "target": "resource"
     },
     {
      "target": "environment",
      "legacy": true
     }
    ]
   }
  ]
 },
 {
  "resourceType": "vpc",
  "driverType": "terraform",
  "module": "vpc",
  "source": {
   "sourceId": "platform",
   "moduleId": "vpc",
   "revision": "v3-2026.10.13",
   "subdirectory": "vpc",
   "contentSha256": "f159d1be486706560730a1d7e36744b5588103d7d6ea9c67e1c120519cb382cc"
  },
  "terraformVersion": "1.9.8",
  "providerRequirements": [
   {
    "name": "aws",
    "source": "registry.terraform.io/hashicorp/aws",
    "version": "6.60.0",
    "binding": "aws"
   }
  ],
  "outputs": [
   {
    "name": "cidr",
    "sensitive": false
   },
   {
    "name": "id",
    "sensitive": false
   },
   {
    "name": "privateSubnetIds",
    "type": "list(any)",
    "sensitive": false
   },
   {
    "name": "subnetIds",
    "type": "list(any)",
    "sensitive": false
   }
  ],
  "variables": [
   {
    "name": "az_count",
    "type": "number",
    "required": false,
    "hasDefault": true,
    "default": 2,
    "resourceParam": false
   },
   {
    "name": "cidr",
    "type": "string",
    "required": false,
    "hasDefault": true,
    "default": "10.42.0.0/16",
    "resourceParam": false
   },
   {
    "name": "name",
    "type": "string",
    "required": true,
    "hasDefault": false,
    "resourceParam": false,
    "preset": "${context.infra.resourceName}"
   },
   {
    "name": "region",
    "type": "string",
    "required": true,
    "hasDefault": false,
    "resourceParam": false
   }
  ],
  "executorOwned": [
   "tags"
  ],
  "account": {
   "required": true,
   "type": "AWS",
   "regionVariable": "region"
  },
  "bindings": [
   {
    "provider": "aws",
    "kind": "aws",
    "targets": [
     {
      "target": "account"
     }
    ]
   }
  ]
 }
];