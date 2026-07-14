// Copyright 2025.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controller

const ReasonBucketCreated = "BucketCreated" // type Normal: new bucket in Garage

const ReasonBucketCreateFailed = "BucketCreateFailed" // type Warning: bucket creation in Garage failed

// RBAC denials in the target namespace.
const (
	ReasonConfigMapAccessForbidden = "ConfigMapAccessForbidden" // type Warning: no ConfigMap access in the namespace
	ReasonSecretAccessForbidden    = "SecretAccessForbidden"    // type Warning: no Secret access in the namespace
)

// rbacRemedyMsg provides additional context for the RBAC failure.
const rbacRemedyMsg = "Grant the controller namespace-access Role and RoleBinding."
