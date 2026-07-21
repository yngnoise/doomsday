import crypto from 'k6/crypto';
import encoding from 'k6/encoding';
import exec from 'k6/execution';
import http from 'k6/http';
import { check, fail, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';
import sse from 'k6/x/sse';

const baseURL = __ENV.BASE_URL || 'http://api:8080';
const dropID = __ENV.LOAD_DROP_ID || 'demo-wraith-jacket';
const jwtSecret = __ENV.LOAD_JWT_SECRET;
const adminPassword = __ENV.LOAD_ADMIN_PASSWORD;
const selected = __ENV.SCENARIO || 'drop-opening';
const sizes = ['XS', 'S', 'M', 'L', 'XL', 'XXL'];

const checkoutCompletion = new Trend('checkout_completion_ms', true);
const checkoutSuccess = new Rate('checkout_success');
const sseFirstEvent = new Trend('sse_first_event_ms', true);
const sseSuccess = new Rate('sse_success');

const scenarioOptions = {
  'drop-opening': {
    dropOpening: {
      executor: 'constant-arrival-rate',
      exec: 'dropOpening',
      rate: 25,
      timeUnit: '1s',
      duration: '20s',
      preAllocatedVUs: 12,
      maxVUs: 40,
    },
  },
  contention: {
    contention: {
      executor: 'shared-iterations',
      exec: 'contention',
      vus: 120,
      iterations: 180,
      maxDuration: '45s',
    },
  },
  checkout: {
    checkout: {
      executor: 'shared-iterations',
      exec: 'checkout',
      vus: 12,
      iterations: 12,
      maxDuration: '30s',
    },
  },
  sse: {
    subscribers: {
      executor: 'per-vu-iterations',
      exec: 'sseSubscriber',
      vus: 20,
      iterations: 1,
      maxDuration: '20s',
    },
    publishers: {
      executor: 'shared-iterations',
      exec: 'contention',
      vus: 20,
      iterations: 20,
      startTime: '2s',
      maxDuration: '15s',
    },
  },
};

if (!scenarioOptions[selected]) {
  throw new Error(`unknown SCENARIO ${selected}`);
}

const thresholds = {
  checks: ['rate>0.98'],
  'http_req_duration{expected_response:true}': ['p(95)<1000'],
  http_req_failed: ['rate<0.02'],
};
if (selected === 'checkout') {
  thresholds.checkout_success = ['rate>0.99'];
  thresholds.checkout_completion_ms = ['p(95)<15000'];
}
if (selected === 'sse') {
  thresholds.sse_success = ['rate>0.99'];
  thresholds.sse_first_event_ms = ['p(95)<5000'];
}

export const options = {
  scenarios: scenarioOptions[selected],
  thresholds,
  summaryTrendStats: ['avg', 'med', 'p(90)', 'p(95)', 'p(99)', 'max'],
};

export function setup() {
  if (!jwtSecret || !adminPassword) {
    fail('LOAD_JWT_SECRET and LOAD_ADMIN_PASSWORD are required');
  }
  const login = http.post(`${baseURL}/api/admin/login`, JSON.stringify({ password: adminPassword }), jsonParams());
  if (!check(login, { 'admin login succeeds': (response) => response.status === 200 })) {
    fail(`admin login failed with ${login.status}`);
  }
  const reset = http.post(`${baseURL}/api/admin/demo/reset`, null, {
    headers: { Authorization: `Bearer ${login.json('token')}` },
  });
  if (!check(reset, { 'demo reset succeeds': (response) => response.status === 200 })) {
    fail(`demo reset failed with ${reset.status}`);
  }
  return { runID: `${selected}-${Date.now()}` };
}

export function dropOpening() {
  const list = http.get(`${baseURL}/api/drops`, { tags: { operation: 'list_drops' } });
  check(list, { 'drop list is available': (response) => response.status === 200 });
  const detail = http.get(`${baseURL}/api/drops/${dropID}`, { tags: { operation: 'get_drop' } });
  check(detail, {
    'live drop is available': (response) => response.status === 200,
    'stock is never negative': (response) => Number(response.json('stock_remaining')) >= 0,
  });
}

export function contention(data) {
  const identity = `${data.runID}-${exec.scenario.name}-${exec.scenario.iterationInTest}-${__VU}`;
  const email = `${identity}@load.invalid`;
  const response = reserve(identity, email, sizes[exec.scenario.iterationInTest % sizes.length]);
  check(response, {
    'contention returns a safe outcome': (result) => [201, 409, 410, 429].includes(result.status),
    'contention never returns a server error': (result) => result.status < 500,
  });
}

export function checkout(data) {
  const started = Date.now();
  const identity = `${data.runID}-checkout-${exec.scenario.iterationInTest}-${__VU}`;
  const email = `${identity}@load.invalid`;
  const token = userToken(identity, email);
  const reservation = reserve(identity, email, sizes[exec.scenario.iterationInTest % sizes.length], token);
  if (!check(reservation, { 'checkout reservation succeeds': (response) => response.status === 201 })) {
    checkoutSuccess.add(false);
    return;
  }
  const reservationID = reservation.json('reservation_id');
  const payment = http.post(
    `${baseURL}/api/checkout/${reservationID}/payments`,
    JSON.stringify({ name: 'Load Test', address: 'Synthetic address', scenario: 'success' }),
    authJSONParams(token, { operation: 'create_payment' }),
  );
  if (!check(payment, { 'payment is accepted': (response) => response.status === 202 })) {
    checkoutSuccess.add(false);
    return;
  }
  const paymentID = payment.json('payment_id');
  let paid = false;
  for (let attempt = 0; attempt < 60; attempt += 1) {
    const status = http.get(`${baseURL}/api/payments/${paymentID}`, authParams(token, { operation: 'get_payment' }));
    if (status.status === 200 && status.json('status') === 'paid') {
      paid = true;
      break;
    }
    sleep(0.25);
  }
  checkoutCompletion.add(Date.now() - started);
  checkoutSuccess.add(paid);
  check(paid, { 'payment reaches paid state': (value) => value === true });
}

export function sseSubscriber() {
  const started = Date.now();
  let received = false;
  const response = sse.open(`${baseURL}/api/drops/${dropID}/events`, { tags: { operation: 'stock_sse' } }, (client) => {
    client.on('event', (event) => {
      const payload = JSON.parse(event.data);
      if (payload.drop_id === dropID && payload.stock >= 0) {
        received = true;
        sseFirstEvent.add(Date.now() - started);
        client.close();
      }
    });
    client.on('error', () => client.close());
  });
  const valid = received && response && response.status === 200;
  sseSuccess.add(valid);
  check(valid, { 'SSE subscriber receives a stock event': (value) => value === true });
}

function reserve(identity, email, size, existingToken) {
  const token = existingToken || userToken(identity, email);
  return http.post(
    `${baseURL}/api/reserve`,
    JSON.stringify({ drop_id: dropID, item_id: 'load-item', size }),
    authJSONParams(token, { operation: 'reserve', expected_response: 'true' }, [201, 409, 410, 429]),
  );
}

function userToken(userID, email) {
  const now = Math.floor(Date.now() / 1000);
  const header = encoding.b64encode(JSON.stringify({ alg: 'HS256', typ: 'JWT' }), 'rawurl');
  const payload = encoding.b64encode(JSON.stringify({ uid: userID, email, role: 'user', iat: now, exp: now + 3600 }), 'rawurl');
  const unsigned = `${header}.${payload}`;
  const signature = encoding.b64encode(crypto.hmac('sha256', jwtSecret, unsigned, 'binary'), 'rawurl');
  return `${unsigned}.${signature}`;
}

function jsonParams() {
  return { headers: { 'Content-Type': 'application/json' } };
}

function authParams(token, tags = {}) {
  return { headers: { Authorization: `Bearer ${token}` }, tags };
}

function authJSONParams(token, tags = {}, expectedStatuses) {
  const params = {
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    tags,
  };
  if (expectedStatuses) {
    params.responseCallback = http.expectedStatuses(...expectedStatuses);
  }
  return params;
}
