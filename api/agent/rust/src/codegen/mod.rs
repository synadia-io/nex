
#![allow(dead_code)]

use serde::{Serialize, Deserialize};
    

/// Error types.
pub mod error {
    /// Error from a TryFrom or FromStr implementation.
    pub struct ConversionError(std::borrow::Cow<'static, str>);
    impl std::error::Error for ConversionError {}
    impl std::fmt::Display for ConversionError {
        fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> Result<(), std::fmt::Error> {
            std::fmt::Display::fmt(&self.0, f)
        }
    }
    impl std::fmt::Debug for ConversionError {
        fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> Result<(), std::fmt::Error> {
            std::fmt::Debug::fmt(&self.0, f)
        }
    }
    impl From<&'static str> for ConversionError {
        fn from(value: &'static str) -> Self {
            Self(value.into())
        }
    }
    impl From<String> for ConversionError {
        fn from(value: String) -> Self {
            Self(value.into())
        }
    }
}
///RegisterAgentRequest
///
/// <details><summary>JSON schema</summary>
///
/// ```json
///{
///  "$id": "https://github.com/synadia-io/nex/agent-api/register-agent-request",
///  "title": "RegisterAgentRequest",
///  "type": "object",
///  "required": [
///    "name",
///    "version"
///  ],
///  "properties": {
///    "description": {
///      "description": "A user friendly description of the agent",
///      "type": "string"
///    },
///    "max_workloads": {
///      "description": "The maximum number of workloads this agent can hold. 0 indicates unlimited",
///      "type": "number"
///    },
///    "name": {
///      "description": "Name of the agent",
///      "type": "string"
///    },
///    "version": {
///      "description": "Version of the agent",
///      "type": "string"
///    }
///  },
///  "additionalProperties": false
///}
/// ```
/// </details>
#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct RegisterAgentRequest {
    ///A user friendly description of the agent
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub max_workloads: Option<f64>,
    ///Name of the agent
    pub name: String,
    ///Version of the agent
    pub version: String,
}
impl From<&RegisterAgentRequest> for RegisterAgentRequest {
    fn from(value: &RegisterAgentRequest) -> Self {
        value.clone()
    }
}
impl RegisterAgentRequest {
    pub fn builder() -> builder::RegisterAgentRequest {
        Default::default()
    }
}
///StartWorkloadRequest
///
/// <details><summary>JSON schema</summary>
///
/// ```json
///{
///  "$id": "https://github.com/synadia-io/nex/agent-api/start-workload-request",
///  "title": "StartWorkloadRequest",
///  "type": "object",
///  "required": [
///    "hash",
///    "name",
///    "namespace",
///    "totalBytes",
///    "workloadId",
///    "workloadType"
///  ],
///  "properties": {
///    "argv": {
///      "description": "Command line arguments to be passed when the workload is a native/service type",
///      "type": "array",
///      "items": {
///        "type": "string"
///      }
///    },
///    "env": {
///      "description": "A map containing environment variables, applicable for native workload types",
///      "type": "object"
///    },
///    "hash": {
///      "description": "A hex encoded SHA-256 hash of the artifact file bytes",
///      "type": "string"
///    },
///    "name": {
///      "description": "Name of the workload",
///      "type": "string"
///    },
///    "namespace": {
///      "description": "Namespace of the workload",
///      "type": "string"
///    },
///    "totalBytes": {
///      "description": "Byte size of the workload artifact",
///      "type": "integer",
///      "minimum": 1.0
///    },
///    "triggerSubjects": {
///      "description": "A list of trigger subjects for the workload, if applicable. Note these are NOT subscribed to by the agent, only used for information and validation",
///      "type": "array",
///      "items": {
///        "type": "string"
///      }
///    },
///    "workloadId": {
///      "description": "The unique identifier of the workload to start.",
///      "type": "string",
///      "format": "uuid"
///    },
///    "workloadType": {
///      "description": "Type of the workload",
///      "type": "string"
///    }
///  },
///  "additionalProperties": false
///}
/// ```
/// </details>
#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct StartWorkloadRequest {
    ///Command line arguments to be passed when the workload is a native/service type
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub argv: Vec<String>,
    ///A map containing environment variables, applicable for native workload types
    #[serde(default, skip_serializing_if = "serde_json::Map::is_empty")]
    pub env: serde_json::Map<String, serde_json::Value>,
    ///A hex encoded SHA-256 hash of the artifact file bytes
    pub hash: String,
    ///Name of the workload
    pub name: String,
    ///Namespace of the workload
    pub namespace: String,
    ///Byte size of the workload artifact
    #[serde(rename = "totalBytes")]
    pub total_bytes: std::num::NonZeroU64,
    ///A list of trigger subjects for the workload, if applicable. Note these are NOT subscribed to by the agent, only used for information and validation
    #[serde(rename = "triggerSubjects", default, skip_serializing_if = "Vec::is_empty")]
    pub trigger_subjects: Vec<String>,
    ///The unique identifier of the workload to start.
    #[serde(rename = "workloadId")]
    pub workload_id: uuid::Uuid,
    ///Type of the workload
    #[serde(rename = "workloadType")]
    pub workload_type: String,
}
impl From<&StartWorkloadRequest> for StartWorkloadRequest {
    fn from(value: &StartWorkloadRequest) -> Self {
        value.clone()
    }
}
impl StartWorkloadRequest {
    pub fn builder() -> builder::StartWorkloadRequest {
        Default::default()
    }
}
///StopWorkloadRequest
///
/// <details><summary>JSON schema</summary>
///
/// ```json
///{
///  "$id": "https://github.com/synadia-io/nex/agent-api/stop-workload-request",
///  "title": "StopWorkloadRequest",
///  "type": "object",
///  "required": [
///    "workloadId"
///  ],
///  "properties": {
///    "immediate": {
///      "description": "Indicates whether the stoppage should be immediate or graceful",
///      "type": "boolean"
///    },
///    "reason": {
///      "description": "Optional reason for stopping the workload",
///      "type": "string"
///    },
///    "workloadId": {
///      "description": "The unique identifier of the workload to stop.",
///      "type": "string",
///      "format": "uuid"
///    }
///  },
///  "additionalProperties": false
///}
/// ```
/// </details>
#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct StopWorkloadRequest {
    ///Indicates whether the stoppage should be immediate or graceful
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub immediate: Option<bool>,
    ///Optional reason for stopping the workload
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub reason: Option<String>,
    ///The unique identifier of the workload to stop.
    #[serde(rename = "workloadId")]
    pub workload_id: uuid::Uuid,
}
impl From<&StopWorkloadRequest> for StopWorkloadRequest {
    fn from(value: &StopWorkloadRequest) -> Self {
        value.clone()
    }
}
impl StopWorkloadRequest {
    pub fn builder() -> builder::StopWorkloadRequest {
        Default::default()
    }
}
/// Types for composing complex structures.
pub mod builder {
    #[derive(Clone, Debug)]
    pub struct RegisterAgentRequest {
        description: Result<Option<String>, String>,
        max_workloads: Result<Option<f64>, String>,
        name: Result<String, String>,
        version: Result<String, String>,
    }
    impl Default for RegisterAgentRequest {
        fn default() -> Self {
            Self {
                description: Ok(Default::default()),
                max_workloads: Ok(Default::default()),
                name: Err("no value supplied for name".to_string()),
                version: Err("no value supplied for version".to_string()),
            }
        }
    }
    impl RegisterAgentRequest {
        pub fn description<T>(mut self, value: T) -> Self
        where
            T: std::convert::TryInto<Option<String>>,
            T::Error: std::fmt::Display,
        {
            self.description = value
                .try_into()
                .map_err(|e| {
                    format!("error converting supplied value for description: {}", e)
                });
            self
        }
        pub fn max_workloads<T>(mut self, value: T) -> Self
        where
            T: std::convert::TryInto<Option<f64>>,
            T::Error: std::fmt::Display,
        {
            self.max_workloads = value
                .try_into()
                .map_err(|e| {
                    format!("error converting supplied value for max_workloads: {}", e)
                });
            self
        }
        pub fn name<T>(mut self, value: T) -> Self
        where
            T: std::convert::TryInto<String>,
            T::Error: std::fmt::Display,
        {
            self.name = value
                .try_into()
                .map_err(|e| format!("error converting supplied value for name: {}", e));
            self
        }
        pub fn version<T>(mut self, value: T) -> Self
        where
            T: std::convert::TryInto<String>,
            T::Error: std::fmt::Display,
        {
            self.version = value
                .try_into()
                .map_err(|e| {
                    format!("error converting supplied value for version: {}", e)
                });
            self
        }
    }
    impl std::convert::TryFrom<RegisterAgentRequest> for super::RegisterAgentRequest {
        type Error = super::error::ConversionError;
        fn try_from(
            value: RegisterAgentRequest,
        ) -> Result<Self, super::error::ConversionError> {
            Ok(Self {
                description: value.description?,
                max_workloads: value.max_workloads?,
                name: value.name?,
                version: value.version?,
            })
        }
    }
    impl From<super::RegisterAgentRequest> for RegisterAgentRequest {
        fn from(value: super::RegisterAgentRequest) -> Self {
            Self {
                description: Ok(value.description),
                max_workloads: Ok(value.max_workloads),
                name: Ok(value.name),
                version: Ok(value.version),
            }
        }
    }
    #[derive(Clone, Debug)]
    pub struct StartWorkloadRequest {
        argv: Result<Vec<String>, String>,
        env: Result<serde_json::Map<String, serde_json::Value>, String>,
        hash: Result<String, String>,
        name: Result<String, String>,
        namespace: Result<String, String>,
        total_bytes: Result<std::num::NonZeroU64, String>,
        trigger_subjects: Result<Vec<String>, String>,
        workload_id: Result<uuid::Uuid, String>,
        workload_type: Result<String, String>,
    }
    impl Default for StartWorkloadRequest {
        fn default() -> Self {
            Self {
                argv: Ok(Default::default()),
                env: Ok(Default::default()),
                hash: Err("no value supplied for hash".to_string()),
                name: Err("no value supplied for name".to_string()),
                namespace: Err("no value supplied for namespace".to_string()),
                total_bytes: Err("no value supplied for total_bytes".to_string()),
                trigger_subjects: Ok(Default::default()),
                workload_id: Err("no value supplied for workload_id".to_string()),
                workload_type: Err("no value supplied for workload_type".to_string()),
            }
        }
    }
    impl StartWorkloadRequest {
        pub fn argv<T>(mut self, value: T) -> Self
        where
            T: std::convert::TryInto<Vec<String>>,
            T::Error: std::fmt::Display,
        {
            self.argv = value
                .try_into()
                .map_err(|e| format!("error converting supplied value for argv: {}", e));
            self
        }
        pub fn env<T>(mut self, value: T) -> Self
        where
            T: std::convert::TryInto<serde_json::Map<String, serde_json::Value>>,
            T::Error: std::fmt::Display,
        {
            self.env = value
                .try_into()
                .map_err(|e| format!("error converting supplied value for env: {}", e));
            self
        }
        pub fn hash<T>(mut self, value: T) -> Self
        where
            T: std::convert::TryInto<String>,
            T::Error: std::fmt::Display,
        {
            self.hash = value
                .try_into()
                .map_err(|e| format!("error converting supplied value for hash: {}", e));
            self
        }
        pub fn name<T>(mut self, value: T) -> Self
        where
            T: std::convert::TryInto<String>,
            T::Error: std::fmt::Display,
        {
            self.name = value
                .try_into()
                .map_err(|e| format!("error converting supplied value for name: {}", e));
            self
        }
        pub fn namespace<T>(mut self, value: T) -> Self
        where
            T: std::convert::TryInto<String>,
            T::Error: std::fmt::Display,
        {
            self.namespace = value
                .try_into()
                .map_err(|e| {
                    format!("error converting supplied value for namespace: {}", e)
                });
            self
        }
        pub fn total_bytes<T>(mut self, value: T) -> Self
        where
            T: std::convert::TryInto<std::num::NonZeroU64>,
            T::Error: std::fmt::Display,
        {
            self.total_bytes = value
                .try_into()
                .map_err(|e| {
                    format!("error converting supplied value for total_bytes: {}", e)
                });
            self
        }
        pub fn trigger_subjects<T>(mut self, value: T) -> Self
        where
            T: std::convert::TryInto<Vec<String>>,
            T::Error: std::fmt::Display,
        {
            self.trigger_subjects = value
                .try_into()
                .map_err(|e| {
                    format!(
                        "error converting supplied value for trigger_subjects: {}", e
                    )
                });
            self
        }
        pub fn workload_id<T>(mut self, value: T) -> Self
        where
            T: std::convert::TryInto<uuid::Uuid>,
            T::Error: std::fmt::Display,
        {
            self.workload_id = value
                .try_into()
                .map_err(|e| {
                    format!("error converting supplied value for workload_id: {}", e)
                });
            self
        }
        pub fn workload_type<T>(mut self, value: T) -> Self
        where
            T: std::convert::TryInto<String>,
            T::Error: std::fmt::Display,
        {
            self.workload_type = value
                .try_into()
                .map_err(|e| {
                    format!("error converting supplied value for workload_type: {}", e)
                });
            self
        }
    }
    impl std::convert::TryFrom<StartWorkloadRequest> for super::StartWorkloadRequest {
        type Error = super::error::ConversionError;
        fn try_from(
            value: StartWorkloadRequest,
        ) -> Result<Self, super::error::ConversionError> {
            Ok(Self {
                argv: value.argv?,
                env: value.env?,
                hash: value.hash?,
                name: value.name?,
                namespace: value.namespace?,
                total_bytes: value.total_bytes?,
                trigger_subjects: value.trigger_subjects?,
                workload_id: value.workload_id?,
                workload_type: value.workload_type?,
            })
        }
    }
    impl From<super::StartWorkloadRequest> for StartWorkloadRequest {
        fn from(value: super::StartWorkloadRequest) -> Self {
            Self {
                argv: Ok(value.argv),
                env: Ok(value.env),
                hash: Ok(value.hash),
                name: Ok(value.name),
                namespace: Ok(value.namespace),
                total_bytes: Ok(value.total_bytes),
                trigger_subjects: Ok(value.trigger_subjects),
                workload_id: Ok(value.workload_id),
                workload_type: Ok(value.workload_type),
            }
        }
    }
    #[derive(Clone, Debug)]
    pub struct StopWorkloadRequest {
        immediate: Result<Option<bool>, String>,
        reason: Result<Option<String>, String>,
        workload_id: Result<uuid::Uuid, String>,
    }
    impl Default for StopWorkloadRequest {
        fn default() -> Self {
            Self {
                immediate: Ok(Default::default()),
                reason: Ok(Default::default()),
                workload_id: Err("no value supplied for workload_id".to_string()),
            }
        }
    }
    impl StopWorkloadRequest {
        pub fn immediate<T>(mut self, value: T) -> Self
        where
            T: std::convert::TryInto<Option<bool>>,
            T::Error: std::fmt::Display,
        {
            self.immediate = value
                .try_into()
                .map_err(|e| {
                    format!("error converting supplied value for immediate: {}", e)
                });
            self
        }
        pub fn reason<T>(mut self, value: T) -> Self
        where
            T: std::convert::TryInto<Option<String>>,
            T::Error: std::fmt::Display,
        {
            self.reason = value
                .try_into()
                .map_err(|e| {
                    format!("error converting supplied value for reason: {}", e)
                });
            self
        }
        pub fn workload_id<T>(mut self, value: T) -> Self
        where
            T: std::convert::TryInto<uuid::Uuid>,
            T::Error: std::fmt::Display,
        {
            self.workload_id = value
                .try_into()
                .map_err(|e| {
                    format!("error converting supplied value for workload_id: {}", e)
                });
            self
        }
    }
    impl std::convert::TryFrom<StopWorkloadRequest> for super::StopWorkloadRequest {
        type Error = super::error::ConversionError;
        fn try_from(
            value: StopWorkloadRequest,
        ) -> Result<Self, super::error::ConversionError> {
            Ok(Self {
                immediate: value.immediate?,
                reason: value.reason?,
                workload_id: value.workload_id?,
            })
        }
    }
    impl From<super::StopWorkloadRequest> for StopWorkloadRequest {
        fn from(value: super::StopWorkloadRequest) -> Self {
            Self {
                immediate: Ok(value.immediate),
                reason: Ok(value.reason),
                workload_id: Ok(value.workload_id),
            }
        }
    }
}
