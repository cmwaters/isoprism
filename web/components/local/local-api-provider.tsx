"use client";

import { useEffect } from "react";

export default function LocalAPIProvider({ apiURL }: { apiURL: string }) {
  useEffect(() => {
    window.__ISOPRISM_API_URL__ = apiURL;
    return () => {
      delete window.__ISOPRISM_API_URL__;
    };
  }, [apiURL]);

  return null;
}
