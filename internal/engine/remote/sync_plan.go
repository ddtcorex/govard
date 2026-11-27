package remote

// The display-only sync plan builder lived here: BuildSyncPlan rendered an rsync
// command string that had drifted from the argv BuildRsyncCommand actually runs
// (it was missing --timeout, --numeric-ids and the SSH options). It is gone: the
// plan a user reads is built from the real command builder, and the fallback
// summary says plainly when the endpoints could not be resolved for it.
