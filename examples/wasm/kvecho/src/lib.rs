#[allow(warnings)]
mod bindings;

use bindings::nexagent::wasm::keyvalue;
use keyvalue::{set, get};

use crate::bindings::exports::nexagent::wasm::nexfunction::Guest;


struct Component;

impl Guest for Component {   
    
    fn run(payload: Vec<u8>) -> Result<Vec<u8>, String> {
        let _ = set("kvecho", "hello", ::std::str::from_utf8(&payload).unwrap());

        let res = get("kvecho", "hello").unwrap();

        let res = format!("hello {}", ::std::str::from_utf8(&res).unwrap());
        Ok(res.as_bytes().to_vec())
    }
}

bindings::export!(Component with_types_in bindings);
