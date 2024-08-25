use std::collections::HashMap;

use super::HostServicesClient;
use crate::Result;

const HEADER_HTTP_URL: &str = "x-http-url";
const HEADER_KEYVALUE_KEY: &str = "x-keyvalue-key";
const HEADER_SUBJECT: &str = "x-subject";
const HEADER_BUCKET: &str = "x-context-bucket";
const HEADER_OBJECT_NAME: &str = "x-object-name";

const SERVICE_NAME_HTTP_CLIENT: &str = "http";
const SERVICE_NAME_MESSAGING: &str = "messaging";
const SERVICE_NAME_OBJECT_STORE: &str = "objectstore";
const SERVICE_NAME_KEYVALUE: &str = "kv";

mod keyvalue;
mod objectstore;
