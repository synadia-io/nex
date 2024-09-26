# Firecracker Image

The files in this directory are what are used to generate the default `rootfs.ext4` file that is used by Firecracker as the rootfs image when launched. You can use the files in this directory as inspiration if you intend to build your own root fs. **NOTE**, however, that your custom root FS must still launch the `nex-agent` as a startup process, otherwise it won't work with Nex nodes.

### Requirements

- docker
- mkfs.ext4
- sudo

### To Generate File

> NOTE: You will be running `sudo go` which will be using root's PATH to find the go binary. This isn't how typical go installations are done, so you will need to check `sudo which go` and see if the binary is found. If not, we have found that soft linking it is the easiest solution. `sudo ln -s /path/to/go/binary/go /usr/local/bin/go`. If you are still getting errors, you may need to manually create roots GOPATH at `/root/go`

`sudo go run . ../path/to/nex-agent`

Should drop `rootfs.ext4.gz` in working directory
