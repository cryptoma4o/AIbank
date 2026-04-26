const CHAT_URL = process.env.CHAT_URL ?? "http://localhost:8106";

export async function sendChatMessage(
  sessionId: string,
  tenantId: string,
  applicationId: string,
  message: string,
  history: Array<{ role: string; content: string }>
) {
  const res = await fetch(`${CHAT_URL}/v1/chat`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      session_id: sessionId,
      tenant_id: tenantId,
      application_id: applicationId,
      stage: "welcome",
      user_message: message,
      history,
    }),
  });
  if (!res.ok) throw new Error("Chat service unavailable");
  return res.json();
}
