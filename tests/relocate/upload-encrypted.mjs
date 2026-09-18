// Produce a genuinely ENCRYPTED entity the way the real web client does:
//   1. ask for an upload session declaring AES-256-CTR support,
//   2. receive key_plain_text + iv,
//   3. encrypt the payload with AES-256-CTR (WebCrypto, same algorithm as the browser),
//   4. upload the ciphertext and record the entity's encryption metadata.
// Exits non-zero on any failure so the caller can budget on it.
const base = process.argv[2];
const token = process.argv[3];
const uri = process.argv[4];
const content = process.argv[5];
const policyId = process.argv[6];

const H = { Authorization: `Bearer ${token}`, "Content-Type": "application/json" };
const b64 = (buf) => Buffer.from(buf).toString("base64");

async function call(path, method, body) {
  const r = await fetch(`${base}${path}`, {
    method,
    headers: H,
    body: body ? JSON.stringify(body) : undefined,
  });
  return await r.json();
}

const session = await call("/api/v4/file/upload", "PUT", {
  uri,
  size: Buffer.byteLength(content, "utf8"),
  mime_type: "text/plain",
  policy_id: policyId,
  encryption_supported: ["aes-256-ctr"],
});

if (!session?.data?.session_id) {
  console.error("session failed:", JSON.stringify(session));
  process.exit(1);
}

const meta = session.data.encrypt_metadata;
if (!meta) {
  console.error("server did not return encrypt_metadata; entity would not be encrypted");
  process.exit(1);
}

const key = Buffer.from(meta.key_plain_text, "base64");
const iv = Buffer.from(meta.iv, "base64");
if (key.length !== 32 || iv.length !== 16) {
  console.error(`unexpected key/iv sizes: key=${key.length} iv=${iv.length}`);
  process.exit(1);
}

const cryptoKey = await crypto.subtle.importKey("raw", key, { name: "AES-CTR" }, false, ["encrypt"]);
const ciphertext = Buffer.from(
  await crypto.subtle.encrypt({ name: "AES-CTR", counter: iv, length: 128 }, cryptoKey, Buffer.from(content, "utf8")),
);

const up = await fetch(`${base}/api/v4/file/upload/${session.data.session_id}/0`, {
  method: "POST",
  headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/octet-stream" },
  body: ciphertext,
});
const upBody = await up.json();
if (upBody.code !== 0) {
  console.error("chunk upload failed:", JSON.stringify(upBody));
  process.exit(1);
}

console.log(
  JSON.stringify({
    policy: session.data.storage_policy?.name,
    ciphertextBytes: ciphertext.length,
    containsPlaintext: ciphertext.includes(Buffer.from(content, "utf8")),
  }),
);
