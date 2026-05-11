import { API_URL } from "@/lib/api";
import { NextRequest, NextResponse } from "next/server";

type RouteContext = {
  params: Promise<{
    userID: string;
    action: string;
  }>;
};

export async function POST(request: NextRequest, context: RouteContext) {
  const { userID, action } = await context.params;
  if (action !== "invite" && action !== "review-email") {
    return new NextResponse("unknown admin action", { status: 404 });
  }

  const password = request.headers.get("X-Admin-Password") ?? "";
  const response = await fetch(
    `${API_URL}/api/v1/admin/pilot/users/${encodeURIComponent(userID)}/${action}`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Admin-Password": password,
      },
    }
  );

  const text = await response.text();
  return new NextResponse(text, {
    status: response.status,
    headers: {
      "Content-Type": response.headers.get("Content-Type") ?? "text/plain; charset=utf-8",
    },
  });
}
