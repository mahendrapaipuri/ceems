---
sidebar_position: 5
---

# Ansible

CEEMS components can be installed and configured using Ansible. The roles are
published to [Ansible Galaxy](https://galaxy.ansible.com/ui/) under the collection
[ceems/ansible](https://galaxy.ansible.com/ui/repo/published/ceems/ansible/).
The collection can be installed using:

```bash
ansible-galaxy collection install ceems.ansible
```

Once the collection is installed, the roles can be used in the playbooks using
`ceems.ansible.ceems_<component>`. For instance, to install `ceems_api_server`,
use the role `ceems.ansible.ceems_api_server`.

The documentation for each role can be found at the following links:

- [CEEMS Exporter Role](https://ceems.github.io/ansible/branch/main/ceems_exporter_role.html)
- [CEEMS API Server Role](https://ceems.github.io/ansible/branch/main/ceems_api_server_role.html)
- [CEEMS Load Balancer Role](https://ceems.github.io/ansible/branch/main/ceems_lb_role.html)
