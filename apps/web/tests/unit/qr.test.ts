// The live ticket's QR encoder (design D7: written here, no dependency). The golden matrices were
// checked when they were recorded by decoding a rendering of each with ZXing (zxing-cpp), which read
// back the exact text at error correction level M; the hashes pin those matrices.
import { createHash } from "node:crypto";
import { expect, it } from "vitest";
import { qrMatrix, qrSvgPath } from "@/lib/tickets/qr";

const CODE = "E1.AQEbTii6L6FNKog_ABbTzKQnbxwaPpt6TFWxsD0fLkpbbAABBwiJgHyHW4SlEUAzzwith2KJkGfeVuN2Sq2x4FgcNXYMABcOFfYvgtIUDVYzxr7tfyDDzaHznxBUQDzivZCG_3at9gU";

const hash = (m: boolean[][]) => createHash("sha256").update(m.map((r) => r.map((c) => (c ? "1" : "0")).join("")).join("\n")).digest("hex");

it("encodes known texts to the matrices ZXing decoded", () => {
  const golden: [string, number, string][] = [
    ["HELLO", 1, "c346c75add5698735afe3f7eb4f3e6c57ccefd9563f45f65c6d76aa518fb6f91"],
    [CODE, 8, "15b0bbc03cd5b4268f97da72f0d47949079ce93437419a64fd837cd95b424b8c"],
    ["https://example.test/a/b?c=1", 3, "98085831b76a9bd63fc92492adb709be63875c832ae6d8cb26e47096f6668361"],
    ["x".repeat(100), 6, "898a791db8daf87950e6fd89cebf33096e6b40e0b49b39bdd015fdd3c07d3ec9"],
  ];
  for (const [text, version, h] of golden) {
    const q = qrMatrix(text);
    expect(q.version, text).toBe(version);
    expect(q.size).toBe(version * 4 + 17);
    expect(hash(q.modules), text).toBe(h);
  }
});

it("a ticket code is a version 8 symbol with its finder patterns, and the SVG has a quiet zone", () => {
  const q = qrMatrix(CODE);
  expect(q.version).toBe(8);
  for (const [x, y] of [[0, 0], [q.size - 7, 0], [0, q.size - 7]]) {
    expect(q.modules[y][x] && q.modules[y + 6][x + 6] && q.modules[y + 3][x + 3] && !q.modules[y + 1][x + 1]).toBe(true);
  }
  const svg = qrSvgPath(CODE);
  expect(svg.viewBox).toBe(q.size + 8);
  expect(svg.path.startsWith("M4 4h1v1h-1z")).toBe(true);
  expect(() => qrMatrix("x".repeat(300))).toThrow();
});
