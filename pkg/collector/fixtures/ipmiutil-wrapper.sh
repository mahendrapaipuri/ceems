#!/bin/bash

echo """ipmiutil dcmi ver 3.17
-- BMC version 6.10, IPMI version 2.0 
DCMI Version:                   1.5
DCMI Power Management:          Supported
DCMI System Interface Access:   Supported
DCMI Serial TMode Access:       Supported
DCMI Secondary LAN Channel:     Supported
  Current Power:                   49 Watts
  Min Power over sample duration:  6 Watts
  Max Power over sample duration:  304 Watts
  Avg Power over sample duration:  49 Watts
  Timestamp:                       Thu Feb 15 09:37:32 2024

  Sampling period:                 1000 ms
  Power reading state is:          active
  Exception Action:  OEM defined
  Power Limit:       896 Watts (inactive)
  Correction Time:   62914560 ms
  Sampling period:   1472 sec
ipmiutil dcmi, completed successfully""" 
