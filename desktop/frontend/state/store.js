const state = {
  environments: [],
  selectedProject: "",
  selectedService: "all",
  selectedSeverity: "all",
  logQuery: "",
  liveLogsEnabled: false,
  terminalModalOpen: false,
  syncConfigs: {},
};

export const getState = () => state;

export const setState = (patch) => {
  Object.assign(state, patch || {});
  return state;
};
