import type { TypeContract } from '../features/platform/moduleMapping';

// Recorded GET /resource-types of the seeded backend (classes are the computed read
// projection), plus the Redis Type a Platform Engineer registers for I04-07.
export const typeFixture: TypeContract[] = [
 {
  "key": "k8s-cluster",
  "inputs": [],
  "outputs": [
   {
    "name": "name",
    "type": "string",
    "required": true
   },
   {
    "name": "endpoint",
    "type": "string"
   },
   {
    "name": "kubeContext",
    "type": "string",
    "required": true
   },
   {
    "name": "kubeconfig",
    "type": "string"
   }
  ],
  "classes": [
   "eks"
  ]
 },
 {
  "key": "k8s-namespace",
  "inputs": [],
  "outputs": [
   {
    "name": "name",
    "type": "string",
    "required": true
   }
  ],
  "classes": []
 },
 {
  "key": "postgres",
  "inputs": [
   {
    "name": "database",
    "type": "string",
    "required": true
   },
   {
    "name": "username",
    "type": "string",
    "required": true
   }
  ],
  "outputs": [
   {
    "name": "host",
    "type": "string",
    "required": true
   },
   {
    "name": "port",
    "type": "number",
    "required": true
   },
   {
    "name": "database",
    "type": "string",
    "required": true
   },
   {
    "name": "username",
    "type": "string",
    "required": true
   },
   {
    "name": "password",
    "type": "string",
    "secret": true,
    "required": true
   }
  ],
  "classes": [
   "in-cluster",
   "cloud-managed"
  ]
 },
 {
  "key": "vpc",
  "inputs": [],
  "outputs": [
   {
    "name": "id",
    "type": "string",
    "required": true
   },
   {
    "name": "cidr",
    "type": "string"
   },
   {
    "name": "subnetIds",
    "type": "any"
   },
   {
    "name": "privateSubnetIds",
    "type": "any"
   }
  ],
  "classes": []
 },
 {
  "key": "workload",
  "inputs": [],
  "outputs": [],
  "classes": []
 },
 {
  "key": "redis",
  "inputs": [],
  "outputs": [
   {
    "name": "host",
    "type": "string",
    "required": true
   },
   {
    "name": "port",
    "type": "number",
    "required": true
   },
   {
    "name": "username",
    "type": "string",
    "required": true
   },
   {
    "name": "password",
    "type": "string",
    "secret": true,
    "required": true
   },
   {
    "name": "tls",
    "type": "bool",
    "required": true
   }
  ],
  "classes": [
   "in-cluster",
   "cloud-managed"
  ]
 }
];
