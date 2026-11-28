// Server-sent events read from a fetch stream (the live dashboard, step 6).

/** Parses server-sent events from a text stream (blocks separated by a blank line). */
export function parseEvents(buffer: string): { events: { event: string; data: string }[]; rest: string } {
  const out: { event: string; data: string }[] = [];
  const parts = buffer.split("\n\n");
  const rest = parts.pop() ?? "";
  for (const block of parts) {
    let event = "message";
    const data: string[] = [];
    for (const line of block.split("\n")) {
      if (line.startsWith(":")) continue;
      if (line.startsWith("event: ")) event = line.slice(7);
      else if (line.startsWith("data: ")) data.push(line.slice(6));
    }
    if (data.length) out.push({ event, data: data.join("\n") });
  }
  return { events: out, rest };
}

