use std::{env, io::{self, Read, Write}};

fn main() {
    let args: Vec<String> = env::args().collect();

    // When a WASI trigger executes:
    // argv[1] is the subject on which it was triggered
    // stdin bytes is the raw input payload
    // stdout bytes is the raw output payload

    let mut buf = Vec::new();
    io::stdin().read_to_end(&mut buf).unwrap();
    
    let mut subject = args[1].as_bytes().to_vec();
    buf.append(&mut subject);

    // This just returns the payload concatenated with the
    // subject
    
    io::stdout().write_all(&mut buf).unwrap();
}