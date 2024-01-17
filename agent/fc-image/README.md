# Firecracker Image
The files in this directory are what are used to generate the default `rootfs.ext4` file that is used by Firecracker as the rootfs image when launched. You can use the files in this directory as inspiration if you intend to build your own root fs. **NOTE**, however, that your custom root FS must still launch the `nex-agent` as a startup process, otherwise it won't work with NEX nodes.

### Requirements
- docker 
-mkfs.ext4
-sudo

### To Generate File

`sudo go run .`

Should drop `rootfs.ext4` in working directory
