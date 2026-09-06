// Exercises src/praxis_dashboard/static/{index.html,app.js,style.css} (T9).
//
// The rest of this project is Python/pytest, but this task's footprint is
// plain browser JS with "no build step" (per the task brief). Node ships a
// built-in test runner (`node:test`) and VM sandboxing (`node:vm`), so no
// npm install / package.json / jsdom dependency is needed to exercise it --
// run with `node --test tests/test_dashboard_static_ui.js`.
//
// app.js is loaded as a classic (non-module) script into a `vm` context with
// a minimal, auto-vivifying fake `document` and a mocked `fetch`/`setInterval`,
// mirroring how the real static/index.html loads it via a plain <script> tag.
// The fake DOM never returns null from getElementById/createElement/
// querySelector (each call lazily fabricates a stub element instead), and
// captures every registered click handler (both addEventListener('click', ..)
// and `.onclick = ..`) into one flat list -- this lets the test drive the
// "Replay" toggle without hard-coding the element id/selector app.js picks
// for it, since the brief does not prescribe index.html's concrete markup.
//
// The fixture snapshot below is deliberately shaped exactly like
// `praxis_dashboard.snapshot.snapshot_to_document`'s output (see
// src/praxis_dashboard/snapshot.py and its DashboardSnapshot dataclass
// fields), with unique marker strings per field so the "does the rendered
// page contain this field's data somewhere" assertions can't pass by
// accident.

'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const STATIC_DIR = path.join(__dirname, '..', 'src', 'praxis_dashboard', 'static');
const INDEX_HTML_PATH = path.join(STATIC_DIR, 'index.html');
const APP_JS_PATH = path.join(STATIC_DIR, 'app.js');
const STYLE_CSS_PATH = path.join(STATIC_DIR, 'style.css');

// Terms the brief bans from every visible label/copy string in this
// footprint (pipeline/bundle-delivery vocabulary and model/vendor names).
const FORBIDDEN_TERMS = [
  /pull request/i,
  /\bPR\b/,
  /\bbranch\b/i,
  /code review/i,
  /tech lead/i,
  /\bbundle\b/i,
  /\bclaude\b/i,
  /\banthropic\b/i,
  /\bopenai\b/i,
  /\bgpt\b/i,
  /\bgemini\b/i,
  /\bllama\b/i,
  /\bmistral\b/i,
  /\bcopilot\b/i,
];

const FIXTURE = {
  mode: 'live',
  run_summary: {
    run_id: 'run-t9-fixture-marker-001',
    total_nodes: 2,
    counts_by_status: { in_progress: 1, blocked: 1 },
    is_complete: false,
  },
  nodes: [
    {
      node_id: 'intake',
      kind: 'task',
      status: 'in_progress',
      legal_next_events: ['complete'],
      is_blocker: false,
      blocked_reason: null,
    },
    {
      node_id: 'review',
      kind: 'task',
      status: 'blocked',
      legal_next_events: [],
      is_blocker: true,
      blocked_reason: 'blocked-reason-marker-aaa',
    },
  ],
  next_actions: [
    'intake can be advanced via: complete',
    'review is blocked: blocked-reason-marker-aaa',
  ],
  evidence: [
    {
      node_id: 'review',
      required_proof_types: ['test-pass'],
      satisfied: false,
      reasons: ['evidence-reason-marker-bbb'],
      stale_warning: 'evidence-stale-warning-marker-ccc',
    },
  ],
  resources: [
    {
      resource_type: 'filesystem',
      identifier: '/workspace/output-marker-ddd.txt',
      owner: 'owner-marker-eee',
      access_mode: 'write',
      epoch: 1,
      expired: true,
      stale_warning: 'lease-stale-warning-marker-fff',
    },
  ],
  executor_assignments: [
    {
      node_id: 'intake',
      proof_type: 'test-pass',
      executor_id: 'executor-marker-ggg',
      grader_kind: 'deterministic',
      status: 'pass',
    },
  ],
  capabilities: [
    {
      executor_id: 'executor-marker-ggg',
      satisfied_kinds: ['test-pass'],
      cost_hint: 4.25,
    },
  ],
  metrics: [
    {
      node_id: 'intake',
      retry_count: 913,
      handoff_count: 717,
      evidence_confidence: { 'test-pass': 0.42 },
    },
  ],
  warnings: ['warning-banner-marker-hhh', 'warning-banner-marker-iii'],
};

function readSource(filePath) {
  return fs.readFileSync(filePath, 'utf8');
}

function makeElement(createdElements, clickHandlers) {
  let onclickHandler = null;
  const el = {
    tagName: 'DIV',
    textContent: '',
    innerHTML: '',
    className: '',
    style: {},
    attributes: {},
    children: [],
    classList: {
      add() {},
      remove() {},
      toggle() {},
      contains() {
        return false;
      },
    },
    appendChild(child) {
      el.children.push(child);
      return child;
    },
    setAttribute(name, value) {
      el.attributes[name] = value;
    },
    getAttribute(name) {
      return Object.prototype.hasOwnProperty.call(el.attributes, name) ? el.attributes[name] : null;
    },
    addEventListener(type, handler) {
      if (type === 'click') clickHandlers.push(handler);
    },
    removeEventListener() {},
    querySelector() {
      return makeElement(createdElements, clickHandlers);
    },
    querySelectorAll() {
      return [];
    },
  };
  Object.defineProperty(el, 'onclick', {
    get() {
      return onclickHandler;
    },
    set(fn) {
      onclickHandler = fn;
      if (typeof fn === 'function') clickHandlers.push(fn);
    },
  });
  createdElements.push(el);
  return el;
}

function buildSandbox(fetchImpl) {
  const createdElements = [];
  const clickHandlers = [];
  const domContentLoadedHandlers = [];
  const intervalCallbacks = [];
  const elementsById = new Map();

  const document = {
    body: makeElement(createdElements, clickHandlers),
    documentElement: makeElement(createdElements, clickHandlers),
    getElementById(id) {
      if (!elementsById.has(id)) {
        elementsById.set(id, makeElement(createdElements, clickHandlers));
      }
      return elementsById.get(id);
    },
    createElement() {
      return makeElement(createdElements, clickHandlers);
    },
    querySelector() {
      return makeElement(createdElements, clickHandlers);
    },
    querySelectorAll() {
      return [];
    },
    addEventListener(type, handler) {
      if (type === 'DOMContentLoaded') domContentLoadedHandlers.push(handler);
    },
  };

  const fetchCalls = [];
  const fetchFn = (url, opts) => {
    fetchCalls.push({ url, opts });
    return fetchImpl(url, opts);
  };

  let intervalId = 0;
  const setIntervalFn = (fn) => {
    intervalCallbacks.push(fn);
    intervalId += 1;
    return intervalId;
  };

  const sandbox = {
    document,
    fetch: fetchFn,
    setInterval: setIntervalFn,
    clearInterval() {},
    setTimeout: global.setTimeout,
    clearTimeout: global.clearTimeout,
    console,
  };
  sandbox.window = sandbox;
  sandbox.globalThis = sandbox;

  const context = vm.createContext(sandbox);

  return {
    context,
    createdElements,
    clickHandlers,
    domContentLoadedHandlers,
    intervalCallbacks,
    fetchCalls,
  };
}

async function flush() {
  for (let i = 0; i < 5; i += 1) {
    // eslint-disable-next-line no-await-in-loop
    await new Promise((resolve) => setImmediate(resolve));
  }
}

function loadAppJsInto(sandboxState) {
  const source = readSource(APP_JS_PATH);
  vm.runInContext(source, sandboxState.context, { filename: APP_JS_PATH });
  for (const handler of sandboxState.domContentLoadedHandlers) handler();
}

test('app.js polls /api/snapshot over GET and flips to the replay query param only after the toggle is used, on the next poll', async () => {
  const fetchImpl = () => Promise.resolve({ ok: true, json: () => Promise.resolve(FIXTURE) });
  const sandboxState = buildSandbox(fetchImpl);

  loadAppJsInto(sandboxState);

  assert.ok(
    sandboxState.intervalCallbacks.length > 0,
    'expected app.js to register its poll loop via setInterval'
  );

  sandboxState.intervalCallbacks[0]();
  await flush();

  assert.ok(sandboxState.fetchCalls.length >= 1, 'expected a poll to fetch("/api/snapshot")');
  assert.equal(sandboxState.fetchCalls[0].url, '/api/snapshot');

  assert.ok(
    sandboxState.clickHandlers.length > 0,
    'expected the Replay toggle button to register a click handler'
  );
  for (const handler of sandboxState.clickHandlers) handler({ preventDefault() {} });

  sandboxState.intervalCallbacks[0]();
  await flush();

  const lastCall = sandboxState.fetchCalls[sandboxState.fetchCalls.length - 1];
  assert.equal(
    lastCall.url,
    '/api/snapshot?replay=1',
    'toggling Replay should flip the query param used by the next poll'
  );

  for (const call of sandboxState.fetchCalls) {
    const method = call.opts && call.opts.method;
    assert.ok(
      method === undefined || String(method).toUpperCase() === 'GET',
      `fetch call to ${call.url} must not use a non-GET method (got ${method})`
    );
    assert.ok(!(call.opts && call.opts.body), `fetch call to ${call.url} must not send a request body`);
  }
});

test("app.js renders every DashboardSnapshot field snapshot_to_document produces somewhere on the page", async () => {
  const fetchImpl = () => Promise.resolve({ ok: true, json: () => Promise.resolve(FIXTURE) });
  const sandboxState = buildSandbox(fetchImpl);

  loadAppJsInto(sandboxState);

  assert.ok(sandboxState.intervalCallbacks.length > 0, 'expected app.js to register a polling interval');
  sandboxState.intervalCallbacks[0]();
  await flush();

  const rendered = sandboxState.createdElements
    .map((el) => `${el.textContent}\n${el.innerHTML}`)
    .join('\n');

  const expectedFragments = [
    FIXTURE.run_summary.run_id,
    FIXTURE.nodes[1].blocked_reason,
    FIXTURE.next_actions[0],
    FIXTURE.evidence[0].reasons[0],
    FIXTURE.evidence[0].stale_warning,
    FIXTURE.resources[0].owner,
    FIXTURE.resources[0].identifier,
    FIXTURE.resources[0].stale_warning,
    FIXTURE.executor_assignments[0].executor_id,
    FIXTURE.capabilities[0].executor_id,
    String(FIXTURE.metrics[0].retry_count),
    String(FIXTURE.metrics[0].handoff_count),
  ];

  for (const fragment of expectedFragments) {
    assert.ok(
      rendered.includes(fragment),
      `expected rendered page output to include ${JSON.stringify(fragment)}`
    );
  }

  for (const warning of FIXTURE.warnings) {
    assert.ok(rendered.includes(warning), `expected the warnings banner to include ${JSON.stringify(warning)}`);
  }
});

test('static UI sources stay in generic runtime vocabulary (no delivery-pipeline or vendor/model terms)', () => {
  const sources = [readSource(INDEX_HTML_PATH), readSource(APP_JS_PATH), readSource(STYLE_CSS_PATH)];
  for (const source of sources) {
    for (const pattern of FORBIDDEN_TERMS) {
      assert.doesNotMatch(source, pattern, `forbidden vocabulary ${pattern} found in a static UI source file`);
    }
  }
});

test('app.js never declares a non-GET fetch method literal', () => {
  const source = readSource(APP_JS_PATH);
  assert.doesNotMatch(
    source,
    /method\s*:\s*['"]?(POST|PUT|DELETE|PATCH)/i,
    'app.js must not declare a non-GET fetch method'
  );
});
