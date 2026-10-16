# ADR 073: Scheduled security reboots

Status: accepted

Unattended package upgrades do not reboot the production host immediately. A
separate timer checks daily at 03:00 Europe/Berlin whether the operating system
has created `/var/run/reboot-required`; only then does it restart the host.

This applies security kernels without indefinitely deferring required reboots
while preserving a predictable maintenance window for the guild website.
