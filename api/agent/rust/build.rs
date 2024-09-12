use std::{fs, path::Path};

use typify::{TypeSpace, TypeSpaceSettings};

fn main() {    
    let content = std::fs::read_to_string("../register-agent-request.json").unwrap();
    let content2 = std::fs::read_to_string("../start-workload-request.json").unwrap();
    let content3 = std::fs::read_to_string("../stop-workload-request.json").unwrap();
    
    let schema = serde_json::from_str(&content).unwrap();
    let schema2 = serde_json::from_str(&content2).unwrap();
    let schema3 = serde_json::from_str(&content3).unwrap();

    let mut type_space = TypeSpace::new(TypeSpaceSettings::default().with_struct_builder(true));    
    type_space.add_root_schema(schema).unwrap();
    type_space.add_root_schema(schema2).unwrap();
    type_space.add_root_schema(schema3).unwrap();

    let contents =
        prettyplease::unparse(&syn::parse2::<syn::File>(type_space.to_stream()).unwrap());

    let preamble = r#"
#![allow(dead_code)]

use serde::{Serialize, Deserialize};
    "#;

    let final_contents = format!("{preamble}\n\n{contents}");

    let mut out_file = Path::new("./src/codegen").to_path_buf();
    out_file.push("mod.rs");
    fs::write(out_file, final_contents).unwrap();
}