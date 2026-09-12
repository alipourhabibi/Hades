import { NextRequest, NextResponse } from 'next/server';

// The session cookie is set here, by the server, so it can carry HttpOnly.
//
// It used to be written from the browser with document.cookie, which meant any
// XSS anywhere in the app could read the session token directly. It also
// appended a literal "; HttpOnly", which browsers ignore on cookies set from
// JavaScript: the flag read as protection and provided none.
//
// The token is never sent back to the browser after this. The /api/rpc proxy
// reads the cookie server-side and attaches the Authorization header.

export const TOKEN_COOKIE = 'hades_token';

const THIRTY_DAYS_SECONDS = 30 * 24 * 60 * 60;

export async function POST(req: NextRequest) {
    let token: unknown;
    try {
        ({ token } = await req.json());
    } catch {
        return NextResponse.json({ message: 'invalid body' }, { status: 400 });
    }
    if (typeof token !== 'string' || token === '') {
        return NextResponse.json({ message: 'token is required' }, { status: 400 });
    }

    const res = NextResponse.json({});
    res.cookies.set({
        name: TOKEN_COOKIE,
        value: token,
        httpOnly: true,
        sameSite: 'lax',
        // Secure whenever the request arrived over TLS. Behind a TLS-terminating
        // proxy the scheme is carried by x-forwarded-proto.
        secure: req.nextUrl.protocol === 'https:' || req.headers.get('x-forwarded-proto') === 'https',
        path: '/',
        maxAge: THIRTY_DAYS_SECONDS,
    });
    return res;
}

export async function DELETE() {
    const res = NextResponse.json({});
    res.cookies.set({ name: TOKEN_COOKIE, value: '', httpOnly: true, path: '/', maxAge: 0 });
    return res;
}
