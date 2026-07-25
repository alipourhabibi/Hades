import { NextRequest, NextResponse } from 'next/server';

const BACKEND_URL = process.env.BACKEND_URL ?? 'http://localhost:50051';

// POST /oauth2/device/registration
//
// The buf CLI calls this endpoint when the user enters a personal access
// token during `buf registry login`. Two cases:
//
//   1. No Authorization header → return 404 so the buf CLI falls back to its
//      "Enter BSR token" prompt (device-code flow is not supported).
//
//   2. Authorization: Bearer <token> → validate the token against the backend
//      and return an OAuth2-style JSON response so the CLI stores the token
//      in ~/.netrc and considers login successful.
export async function POST(req: NextRequest) {
    const authHeader = req.headers.get('authorization');

    if (!authHeader) {
        return new NextResponse(null, { status: 404 });
    }

    let res: Response;
    try {
        res = await fetch(
            `${BACKEND_URL}/buf.alpha.registry.v1alpha1.AuthnService/GetCurrentUser`,
            {
                method: 'POST',
                headers: {
                    'content-type': 'application/json',
                    'connect-protocol-version': '1',
                    authorization: authHeader,
                },
                body: '{}',
            },
        );
    } catch {
        return NextResponse.json({ error: 'backend unreachable' }, { status: 502 });
    }

    if (!res.ok) {
        return NextResponse.json({ error: 'invalid token' }, { status: 401 });
    }

    const token = authHeader.replace(/^Bearer\s+/i, '').trim();
    return NextResponse.json({ access_token: token, token_type: 'bearer' });
}
