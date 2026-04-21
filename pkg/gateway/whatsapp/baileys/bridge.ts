/**
 * WhatsApp bridge using Baileys. Runs as a Bun subprocess.
 * Communicates via JSON lines on stdout.
 *
 * Events emitted:
 *   {"event":"qr","data":"qr-string"}
 *   {"event":"connected","data":{"phone":"...","name":"..."}}
 *   {"event":"message","data":{channel,platform,sender,content,timestamp,raw}}
 *   {"event":"error","data":"error message"}
 *   {"event":"close","data":"reason"}
 */

import makeWASocket, {
  DisconnectReason,
  useMultiFileAuthState,
  makeCacheableSignalKeyStore,
  fetchLatestBaileysVersion,
} from "@whiskeysockets/baileys";
import * as fs from "fs";
import * as path from "path";

const AUTH_DIR = process.env.WA_AUTH_DIR || path.join(process.cwd(), "auth");

function emit(event: string, data: unknown) {
  process.stdout.write(JSON.stringify({ event, data }) + "\n");
}

async function start() {
  if (!fs.existsSync(AUTH_DIR)) fs.mkdirSync(AUTH_DIR, { recursive: true });

  const { state, saveCreds } = await useMultiFileAuthState(AUTH_DIR);
  const { version } = await fetchLatestBaileysVersion();
  emit("info", `Using WA version ${version.join(".")}`);

  const sock = makeWASocket({
    auth: {
      creds: state.creds,
      keys: makeCacheableSignalKeyStore(state.keys, undefined as any),
    },
    version,
    printQRInTerminal: true,
    browser: ["bc", "Desktop", "1.0.0"],
    syncFullHistory: false,
    markOnlineOnConnect: false,
  });

  sock.ev.on("connection.update", (update) => {
    const { connection, lastDisconnect, qr } = update;

    if (qr) {
      emit("qr", qr);
    }

    if (connection === "close") {
      const code = (lastDisconnect?.error as any)?.output?.statusCode;
      if (code === DisconnectReason.loggedOut) {
        emit("close", "logged_out");
        process.exit(1);
      }
      emit("error", `disconnected (code ${code}), reconnecting...`);
      setTimeout(() => start(), 3000);
    }

    if (connection === "open") {
      const phone = sock.user?.id?.split(":")[0] || "unknown";
      emit("connected", { phone, name: sock.user?.name || phone });
    }
  });

  sock.ev.on("creds.update", saveCreds);

  sock.ev.on("messages.upsert", ({ messages, type }) => {
    if (type !== "notify") return;
    for (const msg of messages) {
      if (msg.key.fromMe) continue;
      if (msg.key.remoteJid === "status@broadcast") continue;

      const sender = msg.pushName || msg.key.participant || msg.key.remoteJid || "unknown";
      const isGroup = msg.key.remoteJid?.endsWith("@g.us") || false;
      const channel = isGroup
        ? (msg.key.remoteJid?.replace("@g.us", "") || "group")
        : (msg.key.remoteJid?.replace("@s.whatsapp.net", "") || "dm");

      let content = "";
      const m = msg.message;
      if (m?.conversation) content = m.conversation;
      else if (m?.extendedTextMessage?.text) content = m.extendedTextMessage.text;
      else if (m?.imageMessage) content = m.imageMessage.caption ? `[photo] ${m.imageMessage.caption}` : "[photo]";
      else if (m?.videoMessage) content = m.videoMessage.caption ? `[video] ${m.videoMessage.caption}` : "[video]";
      else if (m?.documentMessage) content = `[document: ${m.documentMessage.fileName || "file"}]`;
      else if (m?.audioMessage) content = "[voice message]";
      else if (m?.stickerMessage) content = "[sticker]";
      else content = "[unsupported]";

      emit("message", { channel, platform: "whatsapp", sender, content, timestamp: new Date().toISOString(), raw: msg });
    }
  });
}

start().catch((err) => { emit("error", String(err)); process.exit(1); });
