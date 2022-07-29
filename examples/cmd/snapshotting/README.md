# Snapshotting demo

This example shows snapshotting in action by sending a marker to a VM via a running process (in this case `sleep 422`), snapshotting the VM, closing it, loading and starting a new machine via the same snapshot, and checking for the marker.

This test requires both KVM and root access.

## Running the test

Run this test by first running

```
sudo -E env PATH=$PATH make all
```

followed by

```
sudo -E env PATH=$PATH go run example_demo.go
```

Alternatively, to do both of the above,
```
sudo -E env PATH=$PATH make run
```

Note the user PATH variable is different from the root user's PATH variable, hence the need for `-E env PATH=$PATH`.

Upon running, the VM logs will be printed to the console, as well as the IP of the VM. It will then show that it is sending the marker (in our case, `sleep 422`).

Afterwards, the snapshot is created and the machine is terminated. The snapshot files are saved in the snapshotssh folder created in the directory.

Then, a new machine is created, booted with the snapshot that was just taken, and the IP of the VM will once again be printed to the console (which should be the same as the last machine). The output of searching for the marker (in our case `ps -aux | grep "sleep 422"`) is then printed to the console and the user can confirm that the snapshot loaded properly.

To run this test more dynamically, you can pause the execution of the program after starting the machine (i.e. after the call to m.Start() and the IP is shown on the screen).

```
err = m.Start()

...

vmIP := m.Cfg.NetworkInterfaces[0].StaticConfiguration.IPConfiguration.IPAddr.IP.String()
fmt.Printf("IP of VM: %v\n", vmIP)
fmt.Scanln() // block, allows you to ssh from another shell

...

err = m.Start()

...

fmt.Println("Snapshot loaded")
fmt.Printf("IP of VM: %v\n", ipToRestore)
fmt.Scanln() // block, allows you to ssh from another shell
```

```
sudo ssh -i root-drive-ssh-key root@[ip]
```

Pressing enter resumes execution of the program.

You can remove dependencies via a simple `make clean`.

```
sudo make clean
```

## Issues

You may encounter an issue where the image does not build properly. This is often indicated via the following near the end of terminal output:

```
umount: /firecracker/build/rootfs/mnt: not mounted.
```

This is due to an issue in Firecracker's devtool command used to dynamically create an image. Fixing this is often as simple as rerunning the command.

```
sudo rm -rf root-drive-with-ssh.img root-drive-ssh-key
sudo make image
```