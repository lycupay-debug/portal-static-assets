import crypto from 'node:crypto';

export function createContentHandlers({ pool, getSessionUser, json, readBody }) {
const contentTypes = new Set(['article', 'tool', 'resource', 'project']);
const contentStatuses = new Set(['draft', 'published', 'unpublished']);
const slugPattern = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;
const contentSelect = `content_id AS "id", title, slug, content_type AS "contentType",
  summary, body, visibility, status, created_at AS "createdAt",
  updated_at AS "updatedAt", published_at AS "publishedAt"`;

function normalizeContentInput(body, partial = false) {
  const input = {};
  const fields = ['title', 'slug', 'contentType', 'summary', 'body', 'visibility'];
  for (const field of fields) {
    if (Object.hasOwn(body, field)) input[field] = body[field];
    else if (!partial && ['title', 'slug', 'contentType', 'body'].includes(field)) input[field] = '';
  }
  if (Object.hasOwn(input, 'title')) input.title = String(input.title ?? '').trim();
  if (Object.hasOwn(input, 'slug')) input.slug = String(input.slug ?? '').trim().toLowerCase();
  if (Object.hasOwn(input, 'summary')) input.summary = String(input.summary ?? '').trim();
  if (Object.hasOwn(input, 'body')) input.body = String(input.body ?? '');
  if (Object.hasOwn(input, 'contentType')) input.contentType = String(input.contentType ?? '');
  if (Object.hasOwn(input, 'visibility')) input.visibility = String(input.visibility ?? '');
  const errors = [];
  if (Object.hasOwn(input, 'title') && (!input.title || [...input.title].length > 200)) errors.push('标题必须为 1 至 200 个字符。');
  if (Object.hasOwn(input, 'slug') && (!slugPattern.test(input.slug) || input.slug.length > 100)) errors.push('别名只能使用小写英文字母、数字和单个连字符。');
  if (Object.hasOwn(input, 'contentType') && !contentTypes.has(input.contentType)) errors.push('内容类型不正确。');
  if (Object.hasOwn(input, 'summary') && [...input.summary].length > 500) errors.push('摘要不能超过 500 个字符。');
  if (Object.hasOwn(input, 'body') && (!input.body.trim() || [...input.body].length > 100000)) errors.push('正文不能为空且不能超过 100000 个字符。');
  if (Object.hasOwn(input, 'visibility') && !['public', 'private'].includes(input.visibility)) errors.push('可见范围不正确。');
  return { input, errors };
}

async function handleContentRequest(req, res, url) {
  const user = await getSessionUser(req);
  if (!user) return json(res, 401, { error: '请先登录后再管理内容。' });

  const path = url.pathname;
  if (path === '/content' && req.method === 'GET') {
    const status = url.searchParams.get('status');
    if (status && !contentStatuses.has(status)) return json(res, 400, { error: '内容状态不正确。' });
    const params = [user.user_id];
    let statusClause = '';
    if (status) { params.push(status); statusClause = ' AND status = $2'; }
    const result = await pool.query(
      `SELECT ${contentSelect} FROM portal.portal_content
       WHERE author_id = $1 AND deleted_at IS NULL${statusClause}
       ORDER BY updated_at DESC, content_id DESC LIMIT 500`,
      params,
    );
    return json(res, 200, { items: result.rows });
  }

  if (path === '/content' && req.method === 'POST') {
    const body = await readBody(req, 512 * 1024);
    const { input, errors } = normalizeContentInput(body);
    if (errors.length) return json(res, 400, { error: errors[0], errors });
    if (!Object.hasOwn(input, 'title') || !Object.hasOwn(input, 'slug') ||
        !Object.hasOwn(input, 'contentType') || !Object.hasOwn(input, 'body')) {
      return json(res, 400, { error: '标题、别名、内容类型和正文为必填项。' });
    }
    input.summary ??= '';
    input.visibility ??= 'private';
    const result = await pool.query(
      `INSERT INTO portal.portal_content
       (author_id, title, slug, content_type, summary, body, visibility)
       VALUES ($1, $2, $3, $4, $5, $6, $7)
       RETURNING ${contentSelect}`,
      [user.user_id, input.title, input.slug, input.contentType, input.summary, input.body, input.visibility],
    );
    return json(res, 201, { item: result.rows[0] });
  }

  const match = path.match(/^\/content\/([0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12})(?:\/(publish|unpublish))?$/i);
  if (!match) return json(res, 404, { error: '未找到内容接口。' });
  const [, id, action] = match;
  if (req.method === 'GET' && !action) {
    const result = await pool.query(
      `SELECT ${contentSelect} FROM portal.portal_content
       WHERE content_id = $1 AND author_id = $2 AND deleted_at IS NULL`,
      [id, user.user_id],
    );
    if (!result.rowCount) return json(res, 404, { error: '内容不存在或无权访问。' });
    return json(res, 200, { item: result.rows[0] });
  }
  if (req.method === 'PATCH' && !action) {
    const body = await readBody(req, 512 * 1024);
    const { input, errors } = normalizeContentInput(body, true);
    if (errors.length) return json(res, 400, { error: errors[0], errors });
    const allowed = Object.keys(input);
    if (!allowed.length) return json(res, 400, { error: '没有可更新的字段。' });
    const columnNames = { title: 'title', slug: 'slug', contentType: 'content_type', summary: 'summary', body: 'body', visibility: 'visibility' };
    const assignments = allowed.map((field, index) => `${columnNames[field]} = $${index + 3}`);
    const values = allowed.map((field) => input[field]);
    const result = await pool.query(
      `UPDATE portal.portal_content SET ${assignments.join(', ')}, updated_at = now()
       WHERE content_id = $1 AND author_id = $2 AND deleted_at IS NULL
       RETURNING ${contentSelect}`,
      [id, user.user_id, ...values],
    );
    if (!result.rowCount) return json(res, 404, { error: '内容不存在或无权访问。' });
    return json(res, 200, { item: result.rows[0] });
  }
  if (req.method === 'DELETE' && !action) {
    const result = await pool.query(
      `UPDATE portal.portal_content SET deleted_at = now(), updated_at = now()
       WHERE content_id = $1 AND author_id = $2 AND deleted_at IS NULL RETURNING content_id`,
      [id, user.user_id],
    );
    if (!result.rowCount) return json(res, 404, { error: '内容不存在或无权访问。' });
    return json(res, 200, { ok: true });
  }
  if (req.method === 'POST' && action === 'publish') {
    const client = await pool.connect();
    try {
      await client.query('BEGIN');
      const existing = await client.query(
        `SELECT ${contentSelect} FROM portal.portal_content
         WHERE content_id = $1 AND author_id = $2 AND deleted_at IS NULL
         FOR UPDATE`,
        [id, user.user_id],
      );
      if (!existing.rowCount) {
        await client.query('ROLLBACK');
        return json(res, 404, { error: '内容不存在或无权访问。' });
      }
      const item = existing.rows[0];
      const validation = normalizeContentInput({
        title: item.title, slug: item.slug, contentType: item.contentType,
        summary: item.summary, body: item.body, visibility: item.visibility,
      });
      if (validation.errors.length) {
        await client.query('ROLLBACK');
        return json(res, 400, { error: validation.errors[0], errors: validation.errors });
      }
      if (item.visibility !== 'public') {
        await client.query('ROLLBACK');
        return json(res, 400, { error: '发布前请先将可见范围设置为公开。' });
      }
      const updated = await client.query(
        `UPDATE portal.portal_content
         SET status = 'published', published_at = now(), updated_at = now()
         WHERE content_id = $1 AND author_id = $2 AND deleted_at IS NULL
         RETURNING ${contentSelect}`,
        [id, user.user_id],
      );
      const eventId = crypto.randomUUID();
      const payload = JSON.stringify({
        title: item.title, slug: item.slug, content_type: item.contentType,
        visibility: item.visibility, summary: item.summary, body: item.body,
      });
      await client.query(
        `INSERT INTO portal.event_inbox (event_id, event_type, payload)
         VALUES ($1::uuid, 'portal.content.validate', $2::jsonb)`,
        [eventId, payload],
      );
      await client.query('COMMIT');
      return json(res, 200, { item: updated.rows[0], eventId });
    } catch (error) {
      await client.query('ROLLBACK').catch(() => {});
      throw error;
    } finally {
      client.release();
    }
  }
  if (req.method === 'POST' && action === 'unpublish') {
    const result = await pool.query(
      `UPDATE portal.portal_content SET status = 'unpublished', updated_at = now()
       WHERE content_id = $1 AND author_id = $2 AND deleted_at IS NULL
       RETURNING ${contentSelect}`,
      [id, user.user_id],
    );
    if (!result.rowCount) return json(res, 404, { error: '内容不存在或无权访问。' });
    return json(res, 200, { item: result.rows[0] });
  }
  return json(res, 405, { error: '不支持的请求方式。' });
}

async function handlePublicContentRequest(req, res, url) {
  if (req.method !== 'GET') return json(res, 405, { error: '不支持的请求方式。' });
  if (url.pathname === '/public/content') {
    const result = await pool.query(
      `SELECT content_id AS "id", title, slug, content_type AS "contentType",
        summary, published_at AS "publishedAt"
       FROM portal.portal_content
       WHERE status = 'published' AND visibility = 'public' AND deleted_at IS NULL
       ORDER BY published_at DESC NULLS LAST, updated_at DESC
       LIMIT 100`,
    );
    return json(res, 200, { items: result.rows });
  }
  const match = url.pathname.match(/^\/public\/content\/([a-z0-9]+(?:-[a-z0-9]+)*)$/);
  if (!match) return json(res, 404, { error: '未找到公开内容。' });
  const result = await pool.query(
    `SELECT content_id AS "id", title, slug, content_type AS "contentType",
      summary, body, visibility, status, created_at AS "createdAt",
      updated_at AS "updatedAt", published_at AS "publishedAt"
     FROM portal.portal_content
     WHERE slug = $1 AND status = 'published' AND visibility = 'public' AND deleted_at IS NULL`,
    [match[1]],
  );
  if (!result.rowCount) return json(res, 404, { error: '内容不存在或尚未公开。' });
  return json(res, 200, { item: result.rows[0] });
}

  return { handleContentRequest, handlePublicContentRequest };
}
