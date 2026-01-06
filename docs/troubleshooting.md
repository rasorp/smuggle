# Troubleshooting
Network configuration and environment issues can sometimes lead to unexpected
behavior when running Smuggle. This section outlines common problems and their
solutions.

## Ubuntu
This section covers common issues encountered when running Smuggle on the Ubuntu
operating system.

### Netlink Mac Address Discovery
Ubuntu 22.04 and later has a known issue within the netlink library tracked as
[issue #993](https://github.com/vishvananda/netlink/issues/993) where a race
condition can lead to inconsistent Mac address detection. It can be avoid by
removing the `/usr/lib/systemd/network/99-default.link` file.

## AWS
This section covers common issues encountered when running Smuggle on AWS
infrastructure.

### EC2 Source/Dest Check
When running Smuggle on AWS EC2 instances using the VXLAN provider, ensure that
the instances are launched with the `source_dest_check` attribute disabled. This
setting is crucial for the proper functioning of VXLAN networking, as it allows
the instances to handle traffic that is not explicitly addressed to them.

To check the `source_dest_check` attribute of an EC2 instance, you can use the
AWS CLI with the following command:
```bash
aws ec2 describe-instance-attribute --instance-id <instance-id> --attribute sourceDestCheck
```
