import {googleVertex} from "@ai-sdk/google-vertex";
import {generateText, tool, stepCountls} from "ai"
import {z} from "zod";

export async function POST(req: Request){
    const{question}=await.req.json();
    const result = await generateTest({
        model:
        googleVertex("gemini-2.5-flash-lite"),
        system:
        "You are diabetes dataset analyst Use the data tool for stastics Explain that associations do not prove causes. this is not a medical advice",
        prompt: question,
        tools: {
            description: "Get the actual dataset summary from go API",
            inputSchema: z.object([]),
            execute: async ()=>{
                const res = await fetch("http://localhost:8080/api/summary")
                if (!res.ok){
                    throw new Error("Could not retrieve dataset summary")
                }
                return res.json()

            },
        },
        stopWhen: stepCountls(3),
    }),

    return Response.json({answer:result.text});

}