import { google } from "@ai-sdk/google";
import { generateText } from "ai";

const API = "http://localhost:8080";

export async function POST(req: Request) {
  try {
    const body = await req.json();
    const question = typeof body.question === "string" ? body.question.trim() : "";

    if (!question) {
      return Response.json({ error: "Question is required" }, { status: 400 });
    }

    const summaryResponse = await fetch(`${API}/api/summary`, {
      cache: "no-store",
    });

    if (!summaryResponse.ok) {
      throw new Error("Could not retrieve dataset summary");
    }

    const summary = await summaryResponse.json();
    const result = await generateText({
      model: google("gemini-2.5-flash"),
      system:
        "You are a diabetes dataset analyst. Explain that associations do not prove causes. This is not medical advice.",
      prompt: `Use this dataset summary to answer the user's question. Do not invent statistics.

Dataset summary:
${JSON.stringify(summary)}

User question:
${question}`,
    });

    return Response.json({ answer: result.text });
  } catch (error) {
    console.error("Gemini request failed:", error);
    const statusCode =
      typeof error === "object" && error !== null && "statusCode" in error
        ? Number(error.statusCode)
        : 500;
    const message =
      statusCode === 402
        ? "Gemini billing credits are depleted. Add credits in Google AI Studio."
        : "Could not generate an answer";

    return Response.json(
      { error: message },
      { status: statusCode === 402 ? 402 : 500 },
    );
  }
}