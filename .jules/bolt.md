## 2025-05-23 - [Docker List Optimization]
**Learning:** ContainerList fetches all containers by default. In a system with many containers, this is slow.
**Action:** Always use filters.Args to narrow down the search when looking for specific containers.
