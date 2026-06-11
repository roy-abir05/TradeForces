import http from "k6/http";
import { check } from "k6";

const engine1 = open("./1.cpp");
const engine2 = open("./2.cpp");

const engines = [engine1, engine2];

export const options = {
  scenarios: {
    spike: {
      executor: "per-vu-iterations",
      vus: 100,
      iterations: 1,
      maxDuration: "1m",
    },
  },
};

export default function () {
  const url = "http://localhost:8080/deploy";

  const randomIndex = Math.floor(Math.random() * engines.length);
  const selectedCode = engines[randomIndex];

  const payload = JSON.stringify({
    submissionId: `stress-${__VU}-${Date.now()}`,
    code: selectedCode,
  });

  const params = {
    headers: { "Content-Type": "application/json" },
  };

  const res = http.post(url, payload, params);

  check(res, {
    "engine accepted (200)": (r) => r.status === 200,
  });
}
