import test from "node:test";
import assert from "node:assert/strict";
import { fmt, parseDollars, waiting } from "../src/format.ts";
import { mockApi } from "../src/mock.ts";

test("money formatting", () => {
  assert.equal(fmt(15000), "$150.00");
  assert.equal(fmt(-250), "-$2.50");
  assert.equal(fmt(0), "$0.00");
  assert.equal(parseDollars("150.00"), 15000);
  assert.equal(parseDollars("150.5"), 15050);
  assert.equal(parseDollars(""), null);
  assert.equal(parseDollars("1.2.3"), null);
  assert.equal(waiting(Date.now() - 4 * 60_000), "waiting 4 min");
});

test("full shift lifecycle in mock mode", async () => {
  let s = await mockApi.state();
  assert.equal(s.shift, null);
  assert.equal(s.tables.length, 6);
  assert.equal(s.requests.length, 2);

  // open shift
  await mockApi.openShift(15000);
  s = await mockApi.state();
  assert.ok(s.shift);
  assert.equal(s.shift.number, "shift-121");
  assert.equal(s.shift.opening_float_cents, 15000);
  assert.equal(s.shift.expected_cents, 15000 + 113450);
  await assert.rejects(() => mockApi.openShift(15000)); // one open shift per till

  // take an order on a free table with a doneness mod
  const free = s.tables.find((t) => t.number === "09")!;
  assert.equal(free.ticket, null);
  await mockApi.addLines(free.id, [
    { menu_item_id: "m-ribeye300", qty: 1, options: ["medium rare"] },
    { menu_item_id: "m-chips", qty: 2, options: [] },
  ]);
  s = await mockApi.state();
  const t09 = s.tables.find((t) => t.number === "09")!;
  assert.ok(t09.ticket);
  assert.equal(t09.ticket.lines.length, 2);
  assert.equal(t09.ticket.fired_at, null);
  const total = t09.ticket.lines.reduce((a, l) => a + l.unit_price_cents * l.qty, 0);
  assert.equal(total, 4600 + 2 * 900);

  // fire
  await mockApi.fire(t09.ticket.id);
  s = await mockApi.state();
  assert.ok(s.tables.find((t) => t.number === "09")!.ticket!.fired_at);

  // requests ack/dismiss
  const waiter = s.requests.find((r) => r.kind === "waiter")!;
  await mockApi.ack(waiter.id);
  const bill = s.requests.find((r) => r.kind === "bill")!;
  await mockApi.dismiss(bill.id);
  s = await mockApi.state();
  assert.equal(s.requests.length, 0);

  // close with a variance
  const expected = s.shift!.expected_cents;
  const posted = await mockApi.closeShift(s.shift!.id, expected - 250);
  assert.equal(posted.declared_cents - posted.expected_cents, -250);
  assert.equal(posted.number, "shift-121");
  s = await mockApi.state();
  assert.equal(s.shift, null); // ready for the next shift

  // next shift gets the next number
  await mockApi.openShift(20000);
  s = await mockApi.state();
  assert.equal(s.shift!.number, "shift-122");
});
