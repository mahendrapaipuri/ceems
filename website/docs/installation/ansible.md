---
sidebar_position: 5
---

# Ansible

CEEMS components can be installed and configured using Ansible. The roles are 
published to [Ansible Galaxy](https://galaxy.ansible.com/ui/) under the collection 
[mahendrapaipuri/ansible](https://galaxy.ansible.com/ui/repo/published/mahendrapaipuri/ansible/).
The collection can be installed using 

```
ansible-galaxy collection install mahendrapaipuri.ansible
```

Once the collection is installed, the roles can be used in the playbooks using 
`mahendrapaipuri.ansible.ceems_<component>`. For instance, to install `ceems_api_server`, 
use the role `mahendrapaipuri.ansible.ceems_api_server`.

The documentation of each role can be found in the following links:

- [CEEMS Exporter Role](https://mahendrapaipuri.github.io/ansible/branch/main/ceems_exporter_role.html)
- [CEEMS API Server](https://mahendrapaipuri.github.io/ansible/branch/main/ceems_api_server_role.html)
- [CEEMS Load Balancer](https://mahendrapaipuri.github.io/ansible/branch/main/ceems_lb_role.html)
