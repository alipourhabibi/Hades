import { NextRequest, NextResponse } from 'next/server';

const BACKEND_URL = process.env.BACKEND_URL || 'http://localhost:50051';

// Hop-by-hop headers must not be forwarded through a proxy.
const HOP_BY_HOP = new Set([
    'connection', 'keep-alive', 'transfer-encoding', 'te',
    'trailer', 'upgrade', 'proxy-authorization', 'proxy-authenticate',
]);

export async function POST(
    req: NextRequest,
    { params }: { params: Promise<{ path: string[] }> },
) {
    const { path } = await params;
    const url = `${BACKEND_URL}/${path.join('/')}`;

    const reqHeaders = new Headers();
    req.headers.forEach((v, k) => {
        const lk = k.toLowerCase();
        // Drop hop-by-hop, host, and accept-encoding (Node fetch auto-decompresses
        // but keeps content-encoding header, causing length mismatch on return).
        if (!HOP_BY_HOP.has(lk) && lk !== 'host' && lk !== 'accept-encoding') {
            reqHeaders.set(k, v);
        }
    });

    let res: Response;
    try {
        res = await fetch(url, {
            method: 'POST',
            headers: reqHeaders,
            body: await req.arrayBuffer(),
        });
    } catch {
        return NextResponse.json({ message: 'Backend unreachable' }, { status: 502 });
    }

    const resHeaders = new Headers();
    res.headers.forEach((v, k) => {
        const lk = k.toLowerCase();
        if (!HOP_BY_HOP.has(lk) && lk !== 'content-encoding' && lk !== 'content-length') {
            resHeaders.set(k, v);
        }
    });

    // Buffer body to avoid streaming issues between Next.js and Caddy over H2.
    const body = await res.arrayBuffer();
    return new NextResponse(body, { status: res.status, headers: resHeaders });
}
