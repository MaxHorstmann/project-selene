Very fun exercise, I genuinely enjoyed going through it!

# Design decisions and architecture

* Started off by adding a few standard tools (curl, jq, nmap..) to the Rover container, then opened a shell on it (via `docker compose exec rover /bin/bash`) and explored manually for a bit.

* Hitting up $GATEWAY_URL...

```
curl -s $GATEWAY_URL | jq
{
  "colony": "Project Selene",
  "established": "2092-01-15",
  "population": 147,
  "status": "nominal",
  "message": "Welcome to the Selene Lunar Colony Administration Gateway. All systems are operational. For colony infrastructure mapping and status inquiries, contact Artemis Core — the colony command and administration hub.",
  "entrypoint": {
    "pod": "artemis",
    "url": "http://localhost:3002",
    "description": "Colony command and administration"
  }
}
```

... I noticed that the entrypoint URL won't work inside the container (localhost will only work on the host), otoh the pod name doesn't have the port number. What we would need is to navigate inside the network is `http://artemis:3002`. Not sure if that was intentional, but let's roll with it for now.

* Once we hit up artemis, we don't get any port numbers anymore:

```
curl -s http://artemis:3002/dependencies | jq
{
  "id": "artemis",
  "dependencies": [
    {
      "pod_id": "helios",
      "resource": "electrical_power",
      "criticality": "high",
      "notes": "Powers command center, data systems, and administrative infrastructure"
    },
    {
      "pod_id": "nexus",
      "resource": "data_routing",
      "criticality": "medium",
      "notes": "Inter-pod and Earth-side communications backbone"
    },
    {
      "pod_id": "aquifer",
      "resource": "potable_water",
      "criticality": "low",
      "notes": "Drinking water and sanitation for administrative personnel"
    }
  ]
}
```

* Alrighty, looks like we'll have to figure out the ports. Two basic options here, we can traverse the hosts and manually scan ports ... or we could use nmap, which has been battle-tested for decades and might work better in larger colonies in the future.

* Parsing nmap (XML) output is a pain, but I recall there's a nice Go package [nmap](https://github.com/Ullaakut/nmap) which does that for us- Go it is for the mapper!

* Copilot did a decent job drafting this for me, I only had to make a few minor edits e.g. to ensure it's crawling all the pod endpoints. It starts by detecting & nmap'ing the subnet (changed the subnet locally from /16 to /24 to reduce the number of hosts to scan- but again, nmap is battle-tested and should perform well in larger networks as well), then traverses the graph of pods along /dependencies and /supplies until no new pods are found, then maps it out.

* For the reporting part, the script just feeds the previously generated map.json to Sonnet 4.6 with a detailed prompt. Obviously tons of room for iteration here, but the initial output is already quite impressive.

# Code!

[Map!](./rover/src/mapper/main.go) 
[Report!](rover/run_reporting.sh)


# What my agent found

* helios and aquifer are *critical* single points of failure. 

* A number of anomalies and concerns were flagged 

* Full report: [report.md](./rover/output/report.md) - let's go over it, it's quite impressive


# What I'd do with more time

* Run this on different (larger) colonies

* Properly visualize the dependency graph.

* Iterate a lot more on the reporting part, mostly by tweaking the prompt
