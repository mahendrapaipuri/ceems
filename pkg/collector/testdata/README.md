# Test Resources

## AMD GPU Partitioning

This is the data we got from a node with 4 MI300 GPUs that has 6 XCDs each.
Based on this data and
[ROCM Device Plugin](https://github.com/ROCm/k8s-device-plugin), we constructed
the device files in `/sys` file system that we use in the tests.

```bash
ls -la /sys/module/amdgpu/drivers/pci:amdgpu/*/drm 
-----------------------------------------------------------------------------
'/sys/module/amdgpu/drivers/pci:amdgpu/0000:02:00.0/drm':
total 0
drwxr-xr-x  4 root root 0 May  7 13:28 .
drwxr-xr-x 14 root root 0 May  7 13:28 ..
drwxr-xr-x  3 root root 0 May  7 13:28 card0
lrwxrwxrwx  1 root root 0 May  7 14:49 controlD64 -> card0
drwxr-xr-x  3 root root 0 May  7 13:32 renderD128

'/sys/module/amdgpu/drivers/pci:amdgpu/0001:02:00.0/drm':
total 0
drwxr-xr-x  4 root root 0 May  7 13:28 .
drwxr-xr-x 13 root root 0 May  7 13:28 ..
drwxr-xr-x  3 root root 0 May  7 13:28 card8
lrwxrwxrwx  1 root root 0 May  7 14:49 controlD72 -> card8
drwxr-xr-x  3 root root 0 May  7 13:32 renderD136

'/sys/module/amdgpu/drivers/pci:amdgpu/0002:02:00.0/drm':
total 0
drwxr-xr-x  4 root root 0 May  7 13:28 .
drwxr-xr-x 13 root root 0 May  7 13:28 ..
drwxr-xr-x  3 root root 0 May  7 13:28 card16
lrwxrwxrwx  1 root root 0 May  7 14:49 controlD80 -> card16
drwxr-xr-x  3 root root 0 May  7 13:32 renderD144

'/sys/module/amdgpu/drivers/pci:amdgpu/0003:02:00.0/drm':
total 0
drwxr-xr-x  4 root root 0 May  7 13:28 .
drwxr-xr-x 13 root root 0 May  7 13:28 ..
drwxr-xr-x  3 root root 0 May  7 13:28 card24
lrwxrwxrwx  1 root root 0 May  7 14:49 controlD88 -> card24
drwxr-xr-x  3 root root 0 May  7 13:32 renderD152
-----------------------------------------------------------------------------




ls -ld /sys/devices/platform/amdgpu_xcp* 
-----------------------------------------------------------------------------
drwxr-xr-x 4 root root 0 May  7 13:28 /sys/devices/platform/amdgpu_xcp.0
drwxr-xr-x 4 root root 0 May  7 13:28 /sys/devices/platform/amdgpu_xcp.1
drwxr-xr-x 4 root root 0 May  7 13:28 /sys/devices/platform/amdgpu_xcp.10
drwxr-xr-x 4 root root 0 May  7 13:28 /sys/devices/platform/amdgpu_xcp.11
drwxr-xr-x 4 root root 0 May  7 13:28 /sys/devices/platform/amdgpu_xcp.12
drwxr-xr-x 4 root root 0 May  7 13:28 /sys/devices/platform/amdgpu_xcp.13
drwxr-xr-x 4 root root 0 May  7 13:28 /sys/devices/platform/amdgpu_xcp.14
drwxr-xr-x 4 root root 0 May  7 13:28 /sys/devices/platform/amdgpu_xcp.15
drwxr-xr-x 4 root root 0 May  7 13:28 /sys/devices/platform/amdgpu_xcp.16
drwxr-xr-x 4 root root 0 May  7 13:28 /sys/devices/platform/amdgpu_xcp.17
drwxr-xr-x 4 root root 0 May  7 13:28 /sys/devices/platform/amdgpu_xcp.18
drwxr-xr-x 4 root root 0 May  7 13:28 /sys/devices/platform/amdgpu_xcp.19
drwxr-xr-x 4 root root 0 May  7 13:28 /sys/devices/platform/amdgpu_xcp.2
drwxr-xr-x 4 root root 0 May  7 13:28 /sys/devices/platform/amdgpu_xcp.20
drwxr-xr-x 4 root root 0 May  7 13:28 /sys/devices/platform/amdgpu_xcp.21
drwxr-xr-x 4 root root 0 May  7 13:28 /sys/devices/platform/amdgpu_xcp.22
drwxr-xr-x 4 root root 0 May  7 13:28 /sys/devices/platform/amdgpu_xcp.23
drwxr-xr-x 4 root root 0 May  7 13:28 /sys/devices/platform/amdgpu_xcp.24
drwxr-xr-x 4 root root 0 May  7 13:28 /sys/devices/platform/amdgpu_xcp.25
drwxr-xr-x 4 root root 0 May  7 13:28 /sys/devices/platform/amdgpu_xcp.26
drwxr-xr-x 4 root root 0 May  7 13:28 /sys/devices/platform/amdgpu_xcp.27
drwxr-xr-x 4 root root 0 May  7 13:28 /sys/devices/platform/amdgpu_xcp.3
drwxr-xr-x 4 root root 0 May  7 13:28 /sys/devices/platform/amdgpu_xcp.4
drwxr-xr-x 4 root root 0 May  7 13:28 /sys/devices/platform/amdgpu_xcp.5
drwxr-xr-x 4 root root 0 May  7 13:28 /sys/devices/platform/amdgpu_xcp.6
drwxr-xr-x 4 root root 0 May  7 13:28 /sys/devices/platform/amdgpu_xcp.7
drwxr-xr-x 4 root root 0 May  7 13:28 /sys/devices/platform/amdgpu_xcp.8
drwxr-xr-x 4 root root 0 May  7 13:28 /sys/devices/platform/amdgpu_xcp.9
-----------------------------------------------------------------------------


** ls -la /sys/devices/platform/amdgpu_xcp.*/drm **
-----------------------------------------------------------------------------
/sys/devices/platform/amdgpu_xcp.0/drm:
total 0
drwxr-xr-x 4 root root 0 May  7 13:28 .
drwxr-xr-x 4 root root 0 May  7 13:28 ..
drwxr-xr-x 3 root root 0 May  7 13:28 card1
drwxr-xr-x 3 root root 0 May  7 13:32 renderD129

/sys/devices/platform/amdgpu_xcp.10/drm:
total 0
drwxr-xr-x 4 root root 0 May  7 13:28 .
drwxr-xr-x 4 root root 0 May  7 13:28 ..
drwxr-xr-x 3 root root 0 May  7 13:28 card12
drwxr-xr-x 3 root root 0 May  7 13:32 renderD140

/sys/devices/platform/amdgpu_xcp.11/drm:
total 0
drwxr-xr-x 4 root root 0 May  7 13:28 .
drwxr-xr-x 4 root root 0 May  7 13:28 ..
drwxr-xr-x 3 root root 0 May  7 13:28 card13
drwxr-xr-x 3 root root 0 May  7 13:32 renderD141

/sys/devices/platform/amdgpu_xcp.12/drm:
total 0
drwxr-xr-x 4 root root 0 May  7 13:28 .
drwxr-xr-x 4 root root 0 May  7 13:28 ..
drwxr-xr-x 3 root root 0 May  7 13:28 card14
drwxr-xr-x 3 root root 0 May  7 13:32 renderD142

/sys/devices/platform/amdgpu_xcp.13/drm:
total 0
drwxr-xr-x 4 root root 0 May  7 13:28 .
drwxr-xr-x 4 root root 0 May  7 13:28 ..
drwxr-xr-x 3 root root 0 May  7 13:28 card15
drwxr-xr-x 3 root root 0 May  7 13:32 renderD143

/sys/devices/platform/amdgpu_xcp.14/drm:
total 0
drwxr-xr-x 4 root root 0 May  7 13:28 .
drwxr-xr-x 4 root root 0 May  7 13:28 ..
drwxr-xr-x 3 root root 0 May  7 13:28 card17
drwxr-xr-x 3 root root 0 May  7 13:32 renderD145

/sys/devices/platform/amdgpu_xcp.15/drm:
total 0
drwxr-xr-x 4 root root 0 May  7 13:28 .
drwxr-xr-x 4 root root 0 May  7 13:28 ..
drwxr-xr-x 3 root root 0 May  7 13:28 card18
drwxr-xr-x 3 root root 0 May  7 13:32 renderD146

/sys/devices/platform/amdgpu_xcp.16/drm:
total 0
drwxr-xr-x 4 root root 0 May  7 13:28 .
drwxr-xr-x 4 root root 0 May  7 13:28 ..
drwxr-xr-x 3 root root 0 May  7 13:28 card19
drwxr-xr-x 3 root root 0 May  7 13:32 renderD147

/sys/devices/platform/amdgpu_xcp.17/drm:
total 0
drwxr-xr-x 4 root root 0 May  7 13:28 .
drwxr-xr-x 4 root root 0 May  7 13:28 ..
drwxr-xr-x 3 root root 0 May  7 13:28 card20
drwxr-xr-x 3 root root 0 May  7 13:32 renderD148

/sys/devices/platform/amdgpu_xcp.18/drm:
total 0
drwxr-xr-x 4 root root 0 May  7 13:28 .
drwxr-xr-x 4 root root 0 May  7 13:28 ..
drwxr-xr-x 3 root root 0 May  7 13:28 card21
drwxr-xr-x 3 root root 0 May  7 13:32 renderD149

/sys/devices/platform/amdgpu_xcp.19/drm:
total 0
drwxr-xr-x 4 root root 0 May  7 13:28 .
drwxr-xr-x 4 root root 0 May  7 13:28 ..
drwxr-xr-x 3 root root 0 May  7 13:28 card22
drwxr-xr-x 3 root root 0 May  7 13:32 renderD150

/sys/devices/platform/amdgpu_xcp.1/drm:
total 0
drwxr-xr-x 4 root root 0 May  7 13:28 .
drwxr-xr-x 4 root root 0 May  7 13:28 ..
drwxr-xr-x 3 root root 0 May  7 13:28 card2
drwxr-xr-x 3 root root 0 May  7 13:32 renderD130

/sys/devices/platform/amdgpu_xcp.20/drm:
total 0
drwxr-xr-x 4 root root 0 May  7 13:28 .
drwxr-xr-x 4 root root 0 May  7 13:28 ..
drwxr-xr-x 3 root root 0 May  7 13:28 card23
drwxr-xr-x 3 root root 0 May  7 13:32 renderD151

/sys/devices/platform/amdgpu_xcp.21/drm:
total 0
drwxr-xr-x 4 root root 0 May  7 13:28 .
drwxr-xr-x 4 root root 0 May  7 13:28 ..
drwxr-xr-x 3 root root 0 May  7 13:28 card25
drwxr-xr-x 3 root root 0 May  7 13:32 renderD153

/sys/devices/platform/amdgpu_xcp.22/drm:
total 0
drwxr-xr-x 4 root root 0 May  7 13:28 .
drwxr-xr-x 4 root root 0 May  7 13:28 ..
drwxr-xr-x 3 root root 0 May  7 13:28 card26
drwxr-xr-x 3 root root 0 May  7 13:32 renderD154

/sys/devices/platform/amdgpu_xcp.23/drm:
total 0
drwxr-xr-x 4 root root 0 May  7 13:28 .
drwxr-xr-x 4 root root 0 May  7 13:28 ..
drwxr-xr-x 3 root root 0 May  7 13:28 card27
drwxr-xr-x 3 root root 0 May  7 13:32 renderD155

/sys/devices/platform/amdgpu_xcp.24/drm:
total 0
drwxr-xr-x 4 root root 0 May  7 13:28 .
drwxr-xr-x 4 root root 0 May  7 13:28 ..
drwxr-xr-x 3 root root 0 May  7 13:28 card28
drwxr-xr-x 3 root root 0 May  7 13:32 renderD156

/sys/devices/platform/amdgpu_xcp.25/drm:
total 0
drwxr-xr-x 4 root root 0 May  7 13:28 .
drwxr-xr-x 4 root root 0 May  7 13:28 ..
drwxr-xr-x 3 root root 0 May  7 13:28 card29
drwxr-xr-x 3 root root 0 May  7 13:32 renderD157

/sys/devices/platform/amdgpu_xcp.26/drm:
total 0
drwxr-xr-x 4 root root 0 May  7 13:28 .
drwxr-xr-x 4 root root 0 May  7 13:28 ..
drwxr-xr-x 3 root root 0 May  7 13:28 card30
drwxr-xr-x 3 root root 0 May  7 13:32 renderD158

/sys/devices/platform/amdgpu_xcp.27/drm:
total 0
drwxr-xr-x 4 root root 0 May  7 13:28 .
drwxr-xr-x 4 root root 0 May  7 13:28 ..
drwxr-xr-x 3 root root 0 May  7 13:28 card31
drwxr-xr-x 3 root root 0 May  7 13:32 renderD159

/sys/devices/platform/amdgpu_xcp.2/drm:
total 0
drwxr-xr-x 4 root root 0 May  7 13:28 .
drwxr-xr-x 4 root root 0 May  7 13:28 ..
drwxr-xr-x 3 root root 0 May  7 13:28 card3
drwxr-xr-x 3 root root 0 May  7 13:32 renderD131

/sys/devices/platform/amdgpu_xcp.3/drm:
total 0
drwxr-xr-x 4 root root 0 May  7 13:28 .
drwxr-xr-x 4 root root 0 May  7 13:28 ..
drwxr-xr-x 3 root root 0 May  7 13:28 card4
drwxr-xr-x 3 root root 0 May  7 13:32 renderD132

/sys/devices/platform/amdgpu_xcp.4/drm:
total 0
drwxr-xr-x 4 root root 0 May  7 13:28 .
drwxr-xr-x 4 root root 0 May  7 13:28 ..
drwxr-xr-x 3 root root 0 May  7 13:28 card5
drwxr-xr-x 3 root root 0 May  7 13:32 renderD133

/sys/devices/platform/amdgpu_xcp.5/drm:
total 0
drwxr-xr-x 4 root root 0 May  7 13:28 .
drwxr-xr-x 4 root root 0 May  7 13:28 ..
drwxr-xr-x 3 root root 0 May  7 13:28 card6
drwxr-xr-x 3 root root 0 May  7 13:32 renderD134

/sys/devices/platform/amdgpu_xcp.6/drm:
total 0
drwxr-xr-x 4 root root 0 May  7 13:28 .
drwxr-xr-x 4 root root 0 May  7 13:28 ..
drwxr-xr-x 3 root root 0 May  7 13:28 card7
drwxr-xr-x 3 root root 0 May  7 13:32 renderD135

/sys/devices/platform/amdgpu_xcp.7/drm:
total 0
drwxr-xr-x 4 root root 0 May  7 13:28 .
drwxr-xr-x 4 root root 0 May  7 13:28 ..
drwxr-xr-x 3 root root 0 May  7 13:28 card9
drwxr-xr-x 3 root root 0 May  7 13:32 renderD137

/sys/devices/platform/amdgpu_xcp.8/drm:
total 0
drwxr-xr-x 4 root root 0 May  7 13:28 .
drwxr-xr-x 4 root root 0 May  7 13:28 ..
drwxr-xr-x 3 root root 0 May  7 13:28 card10
drwxr-xr-x 3 root root 0 May  7 13:32 renderD138

/sys/devices/platform/amdgpu_xcp.9/drm:
total 0
drwxr-xr-x 4 root root 0 May  7 13:28 .
drwxr-xr-x 4 root root 0 May  7 13:28 ..
drwxr-xr-x 3 root root 0 May  7 13:28 card11
drwxr-xr-x 3 root root 0 May  7 13:32 renderD139
-----------------------------------------------------------------------------
```

```bash
cat /sys/class/kfd/kfd/topology/nodes/*/properties
----------------------------------------------
cpu_cores_count 48
simd_count 0
mem_banks_count 1
caches_count 0
io_links_count 4
p2p_links_count 3
cpu_core_id_base 0
simd_id_base 0
max_waves_per_simd 0
lds_size_in_kb 0
gds_size_in_kb 0
num_gws 0
wave_front_size 0
array_count 0
simd_arrays_per_engine 0
cu_per_simd_array 0
simd_per_cu 0
max_slots_scratch_cu 0
gfx_target_version 0
vendor_id 0
device_id 0
location_id 0
domain 0
drm_render_minor 0
hive_id 9034655720432782706
num_sdma_engines 0
num_sdma_xgmi_engines 0
num_sdma_queues_per_engine 0
num_cp_queues 0
max_engine_clk_ccompute 3700
----------------------------------------------
cpu_cores_count 48
simd_count 0
mem_banks_count 1
caches_count 0
io_links_count 4
p2p_links_count 3
cpu_core_id_base 64
simd_id_base 0
max_waves_per_simd 0
lds_size_in_kb 0
gds_size_in_kb 0
num_gws 0
wave_front_size 0
array_count 0
simd_arrays_per_engine 0
cu_per_simd_array 0
simd_per_cu 0
max_slots_scratch_cu 0
gfx_target_version 0
vendor_id 0
device_id 0
location_id 0
domain 0
drm_render_minor 0
hive_id 9034655720432782706
num_sdma_engines 0
num_sdma_xgmi_engines 0
num_sdma_queues_per_engine 0
num_cp_queues 0
max_engine_clk_ccompute 3700
----------------------------------------------
cpu_cores_count 48
simd_count 0
mem_banks_count 1
caches_count 0
io_links_count 4
p2p_links_count 3
cpu_core_id_base 128
simd_id_base 0
max_waves_per_simd 0
lds_size_in_kb 0
gds_size_in_kb 0
num_gws 0
wave_front_size 0
array_count 0
simd_arrays_per_engine 0
cu_per_simd_array 0
simd_per_cu 0
max_slots_scratch_cu 0
gfx_target_version 0
vendor_id 0
device_id 0
location_id 0
domain 0
drm_render_minor 0
hive_id 9034655720432782706
num_sdma_engines 0
num_sdma_xgmi_engines 0
num_sdma_queues_per_engine 0
num_cp_queues 0
max_engine_clk_ccompute 3700
----------------------------------------------
cpu_cores_count 48
simd_count 0
mem_banks_count 1
caches_count 0
io_links_count 4
p2p_links_count 3
cpu_core_id_base 192
simd_id_base 0
max_waves_per_simd 0
lds_size_in_kb 0
gds_size_in_kb 0
num_gws 0
wave_front_size 0
array_count 0
simd_arrays_per_engine 0
cu_per_simd_array 0
simd_per_cu 0
max_slots_scratch_cu 0
gfx_target_version 0
vendor_id 0
device_id 0
location_id 0
domain 0
drm_render_minor 0
hive_id 9034655720432782706
num_sdma_engines 0
num_sdma_xgmi_engines 0
num_sdma_queues_per_engine 0
num_cp_queues 0
max_engine_clk_ccompute 3700
----------------------------------------------
cpu_cores_count 0
simd_count 912
mem_banks_count 1
caches_count 470
io_links_count 4
p2p_links_count 3
cpu_core_id_base 0
simd_id_base 2147487744
max_waves_per_simd 8
lds_size_in_kb 64
gds_size_in_kb 0
num_gws 64
wave_front_size 64
array_count 24
simd_arrays_per_engine 1
cu_per_simd_array 10
simd_per_cu 4
max_slots_scratch_cu 32
gfx_target_version 90402
vendor_id 4098
device_id 29856
location_id 512
domain 0
drm_render_minor 128
hive_id 9034655720432782706
num_sdma_engines 2
num_sdma_xgmi_engines 10
num_sdma_queues_per_engine 8
num_cp_queues 24
max_engine_clk_fcompute 2100
local_mem_size 0
fw_version 138
capability 1013424768
debug_prop 1511
sdma_fw_version 19
unique_id 7843934696581995200
num_xcc 6
max_engine_clk_ccompute 3700
----------------------------------------------
cpu_cores_count 0
simd_count 912
mem_banks_count 1
caches_count 470
io_links_count 4
p2p_links_count 3
cpu_core_id_base 0
simd_id_base 2147487784
max_waves_per_simd 8
lds_size_in_kb 64
gds_size_in_kb 0
num_gws 64
wave_front_size 64
array_count 24
simd_arrays_per_engine 1
cu_per_simd_array 10
simd_per_cu 4
max_slots_scratch_cu 32
gfx_target_version 90402
vendor_id 4098
device_id 29856
location_id 512
domain 1
drm_render_minor 136
hive_id 9034655720432782706
num_sdma_engines 2
num_sdma_xgmi_engines 10
num_sdma_queues_per_engine 8
num_cp_queues 24
max_engine_clk_fcompute 2100
local_mem_size 0
fw_version 138
capability 1013424768
debug_prop 1511
sdma_fw_version 19
unique_id 11940228331418201223
num_xcc 6
max_engine_clk_ccompute 3700
----------------------------------------------
cpu_cores_count 0
simd_count 912
mem_banks_count 1
caches_count 470
io_links_count 4
p2p_links_count 3
cpu_core_id_base 0
simd_id_base 2147487824
max_waves_per_simd 8
lds_size_in_kb 64
gds_size_in_kb 0
num_gws 64
wave_front_size 64
array_count 24
simd_arrays_per_engine 1
cu_per_simd_array 10
simd_per_cu 4
max_slots_scratch_cu 32
gfx_target_version 90402
vendor_id 4098
device_id 29856
location_id 512
domain 2
drm_render_minor 144
hive_id 9034655720432782706
num_sdma_engines 2
num_sdma_xgmi_engines 10
num_sdma_queues_per_engine 8
num_cp_queues 24
max_engine_clk_fcompute 2100
local_mem_size 0
fw_version 138
capability 1013424768
debug_prop 1511
sdma_fw_version 19
unique_id 7304659965365223271
num_xcc 6
max_engine_clk_ccompute 3700
----------------------------------------------
cpu_cores_count 0
simd_count 912
mem_banks_count 1
caches_count 470
io_links_count 4
p2p_links_count 3
cpu_core_id_base 0
simd_id_base 2147487864
max_waves_per_simd 8
lds_size_in_kb 64
gds_size_in_kb 0
num_gws 64
wave_front_size 64
array_count 24
simd_arrays_per_engine 1
cu_per_simd_array 10
simd_per_cu 4
max_slots_scratch_cu 32
gfx_target_version 90402
vendor_id 4098
device_id 29856
location_id 512
domain 3
drm_render_minor 152
hive_id 9034655720432782706
num_sdma_engines 2
num_sdma_xgmi_engines 10
num_sdma_queues_per_engine 8
num_cp_queues 24
max_engine_clk_fcompute 2100
local_mem_size 0
fw_version 138
capability 1013424768
debug_prop 1511
sdma_fw_version 19
unique_id 6885922256455814113
num_xcc 6
max_engine_clk_ccompute 3700
----------------------------------------------
```
