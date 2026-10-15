import http from 'node:http';
import crypto from 'node:crypto';
import fs from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import pg from 'pg';
import bcrypt from 'bcryptjs';
import { createContentHandlers } from '../content-api/handlers.mjs';
import { resolveResponsiveLayout } from './responsive-layout.mjs';

const { Pool } = pg;
const __dirname = path.dirname(fileURLToPath(import.meta.url));
const databaseURLFile = process.env.PERSONAL_PORTAL_DATABASE_URL_FILE || '/usr/local/etc/personal-portal/database-url';
const databaseURL = process.env.PERSONAL_PORTAL_DATABASE_URL || fs.readFileSync(databaseURLFile, 'utf8').trim();
const pool = new Pool({ connectionString: databaseURL, max: 8, connectionTimeoutMillis: 3000, idleTimeoutMillis: 30000 });
const HOST = process.env.AUTH_API_HOST || '127.0.0.1';
const PORT = Number(process.env.AUTH_API_PORT || 8082);
const SESSION_DAYS = 30;
const COOKIE = 'portal_session';
const UPLOAD_DIR = process.env.PERSONAL_PORTAL_UPLOAD_DIR || '/usr/local/www/personal-portal/uploads';
const MAX_IMAGE_BYTES = 8 * 1024 * 1024;
const emailPattern = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
const rateBuckets = new Map();

function json(res, status, body) {
  res.writeHead(status, {
    'Content-Type': 'application/json; charset=utf-8',
    'Cache-Control': 'no-store',
    'X-Content-Type-Options': 'nosniff',
    'Referrer-Policy': 'same-origin',
  });
  res.end(JSON.stringify(body));
}
function readCookie(req, name) {
  const raw = req.headers.cookie || '';
  for (const part of raw.split(';')) {
    const index = part.indexOf('=');
    if (index > 0 && part.slice(0, index).trim() === name) return part.slice(index + 1).trim();
  }
  return '';
}
function setSessionCookie(res, token, maxAge = SESSION_DAYS * 86400) {
  res.setHeader('Set-Cookie', [
    `${COOKIE}=; Path=/api/auth; HttpOnly; Secure; SameSite=Strict; Max-Age=0`,
    `${COOKIE}=${token}; Path=/api; HttpOnly; Secure; SameSite=Strict; Max-Age=${maxAge}`,
  ]);
}
function clearSessionCookie(res) {
  res.setHeader('Set-Cookie', [
    `${COOKIE}=; Path=/api/auth; HttpOnly; Secure; SameSite=Strict; Max-Age=0`,
    `${COOKIE}=; Path=/api; HttpOnly; Secure; SameSite=Strict; Max-Age=0`,
  ]);
}
function tokenHash(token) {
  return crypto.createHash('sha256').update(token).digest('hex');
}
function rateLimit(req, res) {
  const key = `${req.socket.remoteAddress || 'unknown'}:${req.url?.split('?')[0] || '/'}`;
  const now = Date.now();
  let entry = rateBuckets.get(key);
  if (!entry || entry.until <= now) entry = { count: 0, until: now + 15 * 60 * 1000 };
  entry.count += 1;
  rateBuckets.set(key, entry);
  if (rateBuckets.size > 5000) {
    for (const [k, v] of rateBuckets) if (v.until <= now) rateBuckets.delete(k);
  }
  if (entry.count > 12) {
    res.setHeader('Retry-After', String(Math.max(1, Math.ceil((entry.until - now) / 1000))));
    json(res, 429, { error: '操作过于频繁，请稍后再试。' });
    return true;
  }
  return false;
}
async function readBody(req, maxBytes = 16 * 1024) {
  const chunks = [];
  let size = 0;
  for await (const chunk of req) {
    size += chunk.length;
    if (size > maxBytes) throw Object.assign(new Error('body too large'), { statusCode: 413 });
    chunks.push(chunk);
  }
  const raw = Buffer.concat(chunks).toString('utf8');
  if (!raw) return {};
  try {
    const parsed = JSON.parse(raw);
    if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') throw new Error();
    return parsed;
  } catch {
    throw Object.assign(new Error('invalid JSON'), { statusCode: 400 });
  }
}
function safeUser(row) {
  return { id: row.user_id, email: row.email, displayName: row.display_name, createdAt: row.created_at };
}
async function createSession(userId, res) {
  await pool.query('DELETE FROM portal.portal_auth_sessions WHERE expires_at <= now()');
  const token = crypto.randomBytes(32).toString('base64url');
  const hash = tokenHash(token);
  await pool.query(
    `INSERT INTO portal.portal_auth_sessions (user_id, token_hash, expires_at)
     VALUES ($1, $2, now() + ($3 * interval '1 day'))`,
    [userId, hash, SESSION_DAYS],
  );
  setSessionCookie(res, token);
}
async function getSessionUser(req) {
  const token = readCookie(req, COOKIE);
  if (!token || token.length > 128) return null;
  const result = await pool.query(
    `SELECT u.user_id, u.email, u.display_name, u.created_at, s.expires_at
       FROM portal.portal_auth_sessions s
       JOIN portal.portal_users u ON u.user_id = s.user_id
      WHERE s.token_hash = $1 AND s.expires_at > now()`,
    [tokenHash(token)],
  );
  return result.rows[0] || null;
}

const { handleContentRequest, handlePublicContentRequest } = createContentHandlers({ pool, getSessionUser, json, readBody });



const server = http.createServer(async (req, res) => {
  if (!['GET', 'POST', 'PATCH', 'DELETE'].includes(req.method || '')) return json(res, 405, { error: '不支持的请求方式。' });
  const url = new URL(req.url || '/', 'http://127.0.0.1');
  if (url.pathname === '/healthz' && req.method === 'GET') return json(res, 200, { status: 'ok' });
  if (url.pathname === '/layout-policy' && req.method === 'GET') {
    return json(res, 200, resolveResponsiveLayout(url.searchParams));
  }
  if (req.method === 'POST' && ['/auth/register', '/auth/login'].includes(url.pathname) && rateLimit(req, res)) return;
  try {
    if (url.pathname === '/uploads' && req.method === 'POST') {
      const user = await getSessionUser(req);
      if (!user) return json(res, 401, { error: '请先登录后再上传图片。' });
      const body = await readBody(req, 12 * 1024 * 1024);
      const mime = String(body.mime || '').toLowerCase();
      const allowed = new Set(['image/png', 'image/jpeg', 'image/gif', 'image/webp', 'image/avif']);
      if (!allowed.has(mime) || typeof body.data !== 'string' || !/^[A-Za-z0-9+/]*={0,2}$/.test(body.data)) {
        return json(res, 400, { error: '仅支持 PNG、JPEG、GIF、WebP 或 AVIF 图片。' });
      }
      const bytes = Buffer.from(body.data, 'base64');
      if (!bytes.length || bytes.length > MAX_IMAGE_BYTES) return json(res, 413, { error: '图片大小不能超过 8 MB。' });
      const signatures = {
        'image/png': bytes => bytes.length >= 8 && bytes.subarray(0, 8).equals(Buffer.from([137,80,78,71,13,10,26,10])),
        'image/jpeg': bytes => bytes.length >= 3 && bytes[0] === 0xff && bytes[1] === 0xd8 && bytes[2] === 0xff,
        'image/gif': bytes => bytes.length >= 6 && ['GIF87a','GIF89a'].includes(bytes.subarray(0, 6).toString('ascii')),
        'image/webp': bytes => bytes.length >= 12 && bytes.subarray(0, 4).toString('ascii') === 'RIFF' && bytes.subarray(8, 12).toString('ascii') === 'WEBP',
        'image/avif': bytes => bytes.length >= 12 && bytes.subarray(4, 12).toString('ascii').includes('avif'),
      };
      if (!signatures[mime](bytes)) return json(res, 400, { error: '图片文件内容与声明格式不匹配。' });
      const extension = { 'image/png': 'png', 'image/jpeg': 'jpg', 'image/gif': 'gif', 'image/webp': 'webp', 'image/avif': 'avif' }[mime];
      await fs.promises.mkdir(UPLOAD_DIR, { recursive: true });
      const filename = crypto.randomUUID() + '.' + extension;
      await fs.promises.writeFile(path.join(UPLOAD_DIR, filename), bytes, { flag: 'wx', mode: 0o644 });
      return json(res, 201, { url: '/uploads/' + filename, mime, size: bytes.length });
    }
    if (url.pathname === '/public/content' || url.pathname.startsWith('/public/content/')) {
      return await handlePublicContentRequest(req, res, url);
    }
    if (url.pathname === '/content' || url.pathname.startsWith('/content/') || url.pathname === '/resources' || url.pathname.startsWith('/resources/')) {
      return await handleContentRequest(req, res, url);
    }
    if (url.pathname === '/auth/register' && req.method === 'POST') {
      const body = await readBody(req);
      const email = String(body.email || '').trim().toLowerCase();
      const displayName = String(body.displayName || '').trim();
      const password = String(body.password || '');
      if (!emailPattern.test(email) || email.length > 254) return json(res, 400, { error: '请输入有效的邮箱地址。' });
      if (password.length < 10 || password.length > 72 || Buffer.byteLength(password, 'utf8') > 72) return json(res, 400, { error: '密码须为 10 至 72 个字符，且 UTF-8 编码不超过 72 字节。' });
      if (displayName.length > 60) return json(res, 400, { error: '昵称不能超过 60 个字符。' });
      const passwordHash = await bcrypt.hash(password, 12);
      let inserted;
      try {
        inserted = await pool.query(
          `INSERT INTO portal.portal_users (email, display_name, password_hash)
           VALUES ($1, $2, $3)
           RETURNING user_id, email, display_name, created_at`,
          [email, displayName || email.split('@')[0], passwordHash],
        );
      } catch (error) {
        if (error?.code === '23505') return json(res, 409, { error: '该邮箱已注册，请直接登录。' });
        throw error;
      }
      await createSession(inserted.rows[0].user_id, res);
      return json(res, 201, { user: safeUser(inserted.rows[0]) });
    }
    if (url.pathname === '/auth/login' && req.method === 'POST') {
      const body = await readBody(req);
      const email = String(body.email || '').trim().toLowerCase();
      const password = String(body.password || '');
      if (!emailPattern.test(email) || password.length === 0 || password.length > 72) return json(res, 400, { error: '请输入正确的邮箱和密码。' });
      const result = await pool.query(
        'SELECT user_id, email, display_name, password_hash, created_at FROM portal.portal_users WHERE email = $1',
        [email],
      );
      const user = result.rows[0];
      if (!user || !(await bcrypt.compare(password, user.password_hash))) return json(res, 401, { error: '邮箱或密码不正确。' });
      await createSession(user.user_id, res);
      return json(res, 200, { user: safeUser(user) });
    }
    if (url.pathname === '/auth/me' && req.method === 'GET') {
      const user = await getSessionUser(req);
      if (!user) return json(res, 401, { error: '尚未登录。' });
      const remainingSeconds = Math.max(1, Math.floor((new Date(user.expires_at).getTime() - Date.now()) / 1000));
      setSessionCookie(res, readCookie(req, COOKIE), remainingSeconds);
      return json(res, 200, { user: safeUser(user) });
    }
    if (url.pathname === '/auth/logout' && req.method === 'POST') {
      const token = readCookie(req, COOKIE);
      if (token && token.length <= 128) await pool.query('DELETE FROM portal.portal_auth_sessions WHERE token_hash = $1', [tokenHash(token)]);
      clearSessionCookie(res);
      return json(res, 200, { ok: true });
    }
    return json(res, 404, { error: '未找到接口。' });
  } catch (error) {
    if (error?.code === '23505') return json(res, 409, { error: '内容别名已被使用，请换一个别名。' });
    if (error?.statusCode) return json(res, error.statusCode, { error: error.statusCode === 413 ? '请求内容过大。' : '请求格式不正确。' });
    console.error('auth request failed:', error?.message || 'unknown error');
    return json(res, 500, { error: '服务暂时不可用，请稍后再试。' });
  }
});

async function start() {
  await pool.query('SELECT 1');
  server.listen(PORT, HOST, () => console.log(`personal portal auth API listening on ${HOST}:${PORT}`));
}
start().catch((error) => {
  console.error('auth API startup failed:', error?.message || 'unknown error');
  process.exit(1);
});
