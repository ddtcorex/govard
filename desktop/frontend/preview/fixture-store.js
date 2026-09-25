// @ts-check

/** @type {{service:string, method:string, args:any[], result?:any, error?:string}[]} */
let activeFixtures = [];
/** @type {{service:string, method:string, args:any[]}[]} */
let calls = [];

export const setFixtures = (list) => { activeFixtures = list; };
export const getFixtures = () => activeFixtures;
export const recordCall = (entry) => { calls.push(entry); };
export const getCalls = () => [...calls];
export const resetFixtures = () => { activeFixtures = []; };
export const resetCalls = () => { calls = []; };
