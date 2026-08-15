# vectorcore-LCS-win

## Description

VectorCore LCS Client is a native Windows desktop application for submitting 4G LTE location requests to a GMLC (Gateway Mobile Location Centre) over the OMA Mobile Location Protocol (MLP) v3.5 Le interface. 
 speaks MLP's Standard Location Immediate Service (slir/slia) and Historic Location Immediate Service (hlir/hlia).

## Features

- **Location requests** — Locate a subscriber by IMSI or MSISDN, with support for current-position or current-or-last-known location types and a high-priority flag.
- **QoS controls** — Optionally set horizontal and vertical accuracy targets, accuracy class (assured / best effort), and response time (low delay / delay tolerant).
- **Native map view** — Renders the resolved position and its uncertainty circle on a pannable, zoomable OpenStreetMap-tiled map, drawn natively without an embedded browser.
- **Location history** — Query a target's historic fixes over a chosen time window and plot any selected fix on the map.
- **Connection test** — Check GMLC reachability from the status bar before submitting a request.
- **Settings dialog** — Configure the GMLC MLP URL, client credentials, request timeout, and map tile source entirely from within the app.
- **Registry-backed persistence** — All settings are saved to the current user's registry hive (`HKCU\Software\VectorCore\LCSClient`); there is no config file to manage.
- **No console window** — Built as a GUI-subsystem binary, so it runs without a background console popping up.
