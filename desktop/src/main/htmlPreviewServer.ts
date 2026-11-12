import { randomBytes } from "node:crypto";
import { createServer, type Server } from "node:http";

// Preview documents have their own HTTP origin. They never receive application
// credentials or desktop IPC, and browser storage works as it does on a website.
export class HTMLPreviewServer {
  private readonly documents = new Map<string, { path: string; html: string }>();
  private server?: Server;
  private origin?: Promise<string>;
  private disposed = false;

  async publish(id: string, html: string): Promise<string> {
    if (this.disposed) throw new Error("HTML previews are closed.");
    const origin = await this.start();
    if (this.disposed) throw new Error("HTML previews are closed.");
    const path = `/${randomBytes(32).toString("hex")}/index.html`;
    this.documents.set(id, { path, html });
    return origin + path;
  }

  release(id: string): void {
    this.documents.delete(id);
  }

  clear(): void {
    this.documents.clear();
  }

  private start(): Promise<string> {
    if (this.origin) return this.origin;
    const server = createServer((request, response) => {
      const address = server.address();
      const host = address && typeof address !== "string" ? `127.0.0.1:${address.port}` : "";
      const document = [...this.documents.values()].find((entry) => entry.path === request.url);
      if (request.headers.host !== host || !document || !["GET", "HEAD"].includes(request.method ?? "")) {
        response.writeHead(404).end();
        return;
      }
      response.writeHead(200, {
        "Content-Type": "text/html; charset=utf-8",
        "Cache-Control": "no-store",
        "Referrer-Policy": "no-referrer",
        "X-Content-Type-Options": "nosniff",
      });
      response.end(request.method === "HEAD" ? undefined : document.html);
    });
    this.server = server;
    this.origin = new Promise((resolve, reject) => {
      server.once("error", reject);
      server.listen(0, "127.0.0.1", () => {
        const address = server.address();
        if (!address || typeof address === "string") {
          reject(new Error("HTML preview listener is unavailable."));
          return;
        }
        resolve(`http://127.0.0.1:${address.port}`);
      });
    });
    return this.origin;
  }

  dispose(): void {
    this.disposed = true;
    this.clear();
    this.server?.close();
    this.server?.closeAllConnections();
  }
}
