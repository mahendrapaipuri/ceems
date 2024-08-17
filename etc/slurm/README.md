# SLURM epilog and prolog scripts

CEEMS exporter needs to perform few privileged actions to collect certain information of
compute units. An example [systemd service file](https://github.com/mahendrapaipuri/ceems/blob/main/build/package/ceems_exporter/ceems_exporter.service)
provided in the repo shows the linux capabilities necessary for these privileged actions.

If the operators would like to avoid privileges on CEEMS exporter and run it fully in
userland an alternative approach, in SLURM context, is to use Epilog and Prolog scripts
to write the necessary job information to a file that is readable by CEEMS exporter.
This directory provides those scripts that should be used with SLURM.

An example [systemd service file](https://github.com/mahendrapaipuri/ceems/blob/main/init/systemd/ceems_exporter_no_privs.service)
is also provided in the repo that can be used along with these prolog and epilog scripts.

> [!IMPORTANT]
> The CLI arguments `--collector.slurm.job-props-path` and `--collector.slurm.gpu-job-map-path`
are hidden and cannot be seen in `ceems_exporter --help` output. However, these arguments
exists in the exporter and can be used.

Even with such prolog and epilog scripts, operators should grant the user running CEEMS
exporter permissions to run `ipmi-dcmi` command as this command can be executable by only
`root` by default.
