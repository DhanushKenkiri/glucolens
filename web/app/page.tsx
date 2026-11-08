"use client";

import { useEffect, useState } from "react";

const API = "http://localhost:8080";

type Summary = {
  people: number;
  rate: number;
  columns: string[];
};

type Profile = Record<string, { count: number; rate: number }>;

type Similar = {
  count: number;
  rate: number;
};

type Ops = {
  rows: number;
  requests: number;
};

export default function Home() {
  const [summary, setSummary] = useState<Summary | null>(null);
  const [column, setColumn] = useState("smoking_status");
  const [profile, setProfile] = useState<Profile | null>(null);

  const [question, setQuestion] = useState("");
  const [answer, setAnswer] = useState("");

  const [age, setAge] = useState("45");
  const [bmi, setBmi] = useState("28");
  const [similar, setSimilar] = useState<Similar | null>(null);

  const [ops, setOps] = useState<Ops | null>(null);
  
  useEffect(() => {
    fetch(`${API}/api/summary`)
      .then((res) => res.json())
      .then(setSummary);
  }, []);

  async function loadProfile() {
    const res = await fetch(
      `${API}/api/profile?column=${encodeURIComponent(column)}`
    );
    setProfile(await res.json());
  }

  async function ask() {
    const res = await fetch("/api/chat", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ question }),
    });

    const result = await res.json();
    setAnswer(result.answer ?? result.error ?? "No answer was returned");
  }

  async function findSimilar() {
    const url =
      `${API}/api/people?age=${encodeURIComponent(age)}` +
      `&bmi=${encodeURIComponent(bmi)}`;

    const res = await fetch(url);
    setSimilar(await res.json());
  }

   async function loadOps() {
    const res = await fetch(`${API}/api/ops`);
    setOps(await res.json());
  }


  return (
    <main className="mx-auto max-w-4xl space-y-8 p-8">
      <h1 className="text-3xl font-bold">GlucoLens</h1>

      <section className="space-y-3">
        <h2 className="text-xl font-semibold">Explore</h2>

        {summary && (
          <p>
            {summary.people} records · {summary.rate.toFixed(1)}% diagnosed
          </p>
        )}

        <select
          className="rounded border p-2 bg-gray-900 p-2 text-black"
          value={column}
          onChange={(e) => setColumn(e.target.value)}
        >
          {summary?.columns
            .filter((name: string) => name !== "diagnosed_diabetes")
            .map((name: string) => (
              <option key={name} value={name}>
                {name}
              </option>
            ))}
        </select>

        <button className="rounded bg-black p-2 text-white" onClick={loadProfile}>
          Analyse column
        </button>

        {profile && (
          <pre className="overflow-auto rounded bg-black-100 p-3 text-sm">
            {JSON.stringify(profile, null, 2)}
          </pre>
        )}
      </section>

      <section className="space-y-3">
        <h2 className="text-xl font-semibold">Ask the Data</h2>

        <input
          className="w-full rounded border p-2"
          value={question}
          onChange={(e) => setQuestion(e.target.value)}
          placeholder="Ask a question about the dataset"
        />

        <button className="rounded bg-black p-2 text-white" onClick={ask}>
          Ask Gemini
        </button>

        {answer && <p>{answer}</p>}
      </section>

      <section className="space-y-3">
        <h2 className="text-xl font-semibold">People Like Me</h2>

        <input
          className="rounded border p-2"
          value={age}
          onChange={(e) => setAge(e.target.value)}
          placeholder="Age"
        />

        <input
          className="rounded border p-2"
          value={bmi}
          onChange={(e) => setBmi(e.target.value)}
          placeholder="BMI"
        />

        <button
          className="rounded bg-black p-2 text-white"
          onClick={findSimilar}
        >
          Find similar records
        </button>

        {similar && (
          <p>
            {similar.count} matching records · {similar.rate.toFixed(1)}% diagnosed
          </p>
        )}
      </section>

      <section className="space-y-3">
        <h2 className="text-xl font-semibold">Ops</h2>

        <button className="rounded border p-2" onClick={loadOps}>
          Refresh status
        </button>

        {ops && (
          <p>
            {ops.rows} dataset rows · {ops.requests} API requests
          </p>
        )}
      </section>
    </main>
  );
}