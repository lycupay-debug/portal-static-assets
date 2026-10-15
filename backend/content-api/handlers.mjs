import crypto from 'node:crypto';

export function createContentHandlers({ pool, getSessionUser, json, readBody }) {
const contentTypes = new Set(['article', 'tool', 'resource', 'project']);
const contentStatuses = new Set(['draft', 'published', 'unpublished']);
const contentTags = new Set(['教育','学前教育','小学教育','中学教育','高等教育','职业教育','家庭教育','教学资源','旅游','住宿','交通','景点攻略','旅行规划','美食体验','网络','网络安全','网络维护','网络基础知识','网络诊断','云与服务器','系统','操作系统','系统配置','系统维护','部署运维','故障排查','编程','前端开发','后端开发','Python','JavaScript / TypeScript','数据库','API 与集成','代码实践','软件工具','效率工具','开发工具','文档工具','AI 工具','自动化工具']);
const slugPattern = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;
const contentSelect = `content_id AS "id", title, slug, content_type AS "contentType",
  tags, summary, body, visibility, status, created_at AS "createdAt",
  updated_at AS "updatedAt", published_at AS "publishedAt"`;

function normalizeContentInput(body, partial = false) {
  const input = {};
  const fields = ['title', 'slug', 'contentType', 'tags', 'summary', 'body', 'visibility'];
  for (const field of fields) {
    if (Object.hasOwn(body, field)) input[field] = body[field];
    else if (!partial && ['title', 'slug', 'contentType', 'body', 'tags'].includes(field)) input[field] = '';
  }
  if (Object.hasOwn(input, 'title')) input.title = String(input.title ?? '').trim();
  if (Object.hasOwn(input, 'slug')) input.slug = String(input.slug ?? '').trim().toLowerCase();
  if (Object.hasOwn(input, 'summary')) input.summary = String(input.summary ?? '').trim();
  if (Object.hasOwn(input, 'body')) input.body = String(input.body ?? '');
  if (Object.hasOwn(input, 'contentType')) input.contentType = String(input.contentType ?? '');
  if (Object.hasOwn(input, 'tags')) {
    if (!Array.isArray(input.tags)) input.tags = [];
    input.tags = [...new Set(input.tags.map(tag => String(tag).trim()))];
  }
  if (Object.hasOwn(input, 'visibility')) input.visibility = String(input.visibility ?? '');
  const errors = [];
  if (Object.hasOwn(input, 'title') && (!input.title || [...input.title].length > 200)) errors.push('标题必须为 1 至 200 个字符。');
  if (Object.hasOwn(input, 'slug') && (!slugPattern.test(input.slug) || input.slug.length > 100)) errors.push('别名只能使用小写英文字母、数字和单个连字符。');
  if (Object.hasOwn(input, 'contentType') && !contentTypes.has(input.contentType)) errors.push('内容类型不正确。');
  if (Object.hasOwn(input, 'tags') && (input.tags.length > 40 || input.tags.some(tag => !contentTags.has(tag)))) errors.push('知识广场标签不正确，请从现有标签中选择。');
  if (Object.hasOwn(input, 'summary') && [...input.summary].length > 500) errors.push('摘要不能超过 500 个字符。');
  if (Object.hasOwn(input, 'body') && (!input.body.trim() || [...input.body].length > 100000)) errors.push('正文不能为空且不能超过 100000 个字符。');
  if (Object.hasOwn(input, 'visibility') && !['public', 'private'].includes(input.visibility)) errors.push('可见范围不正确。');
  return { input, errors };
}

async function saveContentVersion(client, item, authorId) {
  const next = await client.query(
    `SELECT COALESCE(MAX(version_number), 0) + 1 AS next_version
     FROM portal.portal_content_versions WHERE content_id = $1`, [item.id]);
  await client.query(
    `INSERT INTO portal.portal_content_versions (content_id, author_id, version_number, snapshot)
     VALUES ($1, $2, $3, $4::jsonb)`,
    [item.id, authorId, next.rows[0].next_version, JSON.stringify(item)]);
}

async function handleContentRequest(req, res, url) {
  const user = await getSessionUser(req);
  if (!user) return json(res, 401, { error: '请先登录后再管理内容。' });

  const path = url.pathname;
  if (path === '/content/trash' && req.method === 'GET') {
    const result = await pool.query(
      `SELECT ${contentSelect}, deleted_at AS \"deletedAt\"
       FROM portal.portal_content WHERE author_id = $1 AND deleted_at IS NOT NULL
       ORDER BY deleted_at DESC, content_id DESC LIMIT 500`, [user.user_id]);
    return json(res, 200, { items: result.rows });
  }
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
    if (!input.tags?.length) return json(res, 400, { error: '请至少选择一个知识广场标签。' });
    input.summary ??= '';
    input.visibility ??= 'private';
    const result = await pool.query(
      `INSERT INTO portal.portal_content
       (author_id, title, slug, content_type, tags, summary, body, visibility)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
       RETURNING ${contentSelect}`,
      [user.user_id, input.title, input.slug, input.contentType, input.tags, input.summary, input.body, input.visibility],
    );
    return json(res, 201, { item: result.rows[0] });
  }

  const uuid = '[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}';
  const versionRestoreMatch = path.match(new RegExp(`^/content/(${uuid})/versions/(${uuid})/restore$`, 'i'));
  const versionsMatch = path.match(new RegExp(`^/content/(${uuid})/versions$`, 'i'));
  const recycleRestoreMatch = path.match(new RegExp(`^/content/(${uuid})/restore$`, 'i'));
  const permanentDeleteMatch = path.match(new RegExp(`^/content/(${uuid})/permanent$`, 'i'));
  if (versionsMatch && req.method === 'GET') {
    const id = versionsMatch[1];
    const owned = await pool.query(
      `SELECT content_id FROM portal.portal_content WHERE content_id = $1 AND author_id = $2`, [id, user.user_id]);
    if (!owned.rowCount) return json(res, 404, { error: '内容不存在或无权访问。' });
    const result = await pool.query(
      `SELECT version_id AS \"versionId\", version_number AS \"versionNumber\",
        snapshot->>'title' AS title, snapshot->>'slug' AS slug,
        snapshot->>'status' AS status, created_at AS \"createdAt\"
       FROM portal.portal_content_versions WHERE content_id = $1 AND author_id = $2
       ORDER BY version_number DESC LIMIT 100`, [id, user.user_id]);
    return json(res, 200, { items: result.rows });
  }
  if (recycleRestoreMatch && req.method === 'POST') {
    try {
      const result = await pool.query(
        `UPDATE portal.portal_content SET deleted_at = NULL, updated_at = now()
         WHERE content_id = $1 AND author_id = $2 AND deleted_at IS NOT NULL
         RETURNING ${contentSelect}`, [recycleRestoreMatch[1], user.user_id]);
      if (!result.rowCount) return json(res, 404, { error: '回收站中不存在该内容。' });
      return json(res, 200, { item: result.rows[0] });
    } catch (error) {
      if (error?.code === '23505') return json(res, 409, { error: '恢复失败：该 URL 别名已被其他内容使用，请先调整别名。' });
      throw error;
    }
  }
  if (permanentDeleteMatch && req.method === 'DELETE') {
    const result = await pool.query(
      `DELETE FROM portal.portal_content WHERE content_id = $1 AND author_id = $2 AND deleted_at IS NOT NULL RETURNING content_id`,
      [permanentDeleteMatch[1], user.user_id]);
    if (!result.rowCount) return json(res, 404, { error: '回收站中不存在该内容。' });
    return json(res, 200, { ok: true });
  }
  if (versionRestoreMatch && req.method === 'POST') {
    const [, id, versionId] = versionRestoreMatch;
    const client = await pool.connect();
    try {
      await client.query('BEGIN');
      const current = await client.query(
        `SELECT ${contentSelect} FROM portal.portal_content
         WHERE content_id = $1 AND author_id = $2 AND deleted_at IS NULL FOR UPDATE`, [id, user.user_id]);
      if (!current.rowCount) {
        await client.query('ROLLBACK');
        return json(res, 404, { error: '内容不存在或无权访问。' });
      }
      const version = await client.query(
        `SELECT snapshot FROM portal.portal_content_versions
         WHERE version_id = $1 AND content_id = $2 AND author_id = $3`, [versionId, id, user.user_id]);
      if (!version.rowCount) {
        await client.query('ROLLBACK');
        return json(res, 404, { error: '历史版本不存在。' });
      }
      const snapshot = version.rows[0].snapshot;
      const validation = normalizeContentInput(snapshot);
      if (validation.errors.length) {
        await client.query('ROLLBACK');
        return json(res, 400, { error: '历史版本数据未通过校验，无法恢复。' });
      }
      await saveContentVersion(client, current.rows[0], user.user_id);
      const restored = await client.query(
        `UPDATE portal.portal_content SET title = $3, slug = $4, content_type = $5, tags = $6,
          summary = $7, body = $8, visibility = $9, updated_at = now()
         WHERE content_id = $1 AND author_id = $2 RETURNING ${contentSelect}`,
        [id, user.user_id, snapshot.title, snapshot.slug, snapshot.contentType, snapshot.tags || [], snapshot.summary,
          snapshot.body, current.rows[0].status === 'published' ? 'public' : snapshot.visibility]);
      let eventId = null;
      if (restored.rows[0].status === 'published') {
        eventId = crypto.randomUUID();
        const item = restored.rows[0];
        await client.query(
          `INSERT INTO portal.event_inbox (event_id, event_type, payload) VALUES ($1::uuid, 'portal.content.validate', $2::jsonb)`,
          [eventId, JSON.stringify({ title: item.title, slug: item.slug, content_type: item.contentType, tags: item.tags || [],
            visibility: item.visibility, summary: item.summary, body: item.body })]);
      }
      await client.query('COMMIT');
      return json(res, 200, { item: restored.rows[0], eventId });
    } catch (error) {
      await client.query('ROLLBACK').catch(() => {});
      if (error?.code === '23505') return json(res, 409, { error: '恢复失败：该 URL 别名已被其他内容使用。' });
      throw error;
    } finally { client.release(); }
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
    const columnNames = { title: 'title', slug: 'slug', contentType: 'content_type', tags: 'tags', summary: 'summary', body: 'body', visibility: 'visibility' };
    const client = await pool.connect();
    try {
      await client.query('BEGIN');
      const existing = await client.query(
        `SELECT ${contentSelect} FROM portal.portal_content WHERE content_id = $1 AND author_id = $2 AND deleted_at IS NULL FOR UPDATE`,
        [id, user.user_id]);
      if (!existing.rowCount) {
        await client.query('ROLLBACK');
        return json(res, 404, { error: '内容不存在或无权访问。' });
      }
      const current = existing.rows[0];
      const changed = allowed.filter((field) => Array.isArray(input[field]) && Array.isArray(current[field])
        ? JSON.stringify(input[field]) !== JSON.stringify(current[field])
        : input[field] !== current[field]);
      if (!changed.length) {
        await client.query('COMMIT');
        return json(res, 200, { item: current, versionCreated: false });
      }
      await saveContentVersion(client, current, user.user_id);
      const assignments = changed.map((field, index) => `${columnNames[field]} = $${index + 3}`);
      const values = changed.map((field) => input[field]);
      const result = await client.query(
        `UPDATE portal.portal_content SET ${assignments.join(', ')}, updated_at = now()
         WHERE content_id = $1 AND author_id = $2 AND deleted_at IS NULL RETURNING ${contentSelect}`,
        [id, user.user_id, ...values]);
      await client.query('COMMIT');
      return json(res, 200, { item: result.rows[0], versionCreated: true });
    } catch (error) {
      await client.query('ROLLBACK').catch(() => {});
      if (error?.code === '23505') return json(res, 409, { error: '保存失败：该 URL 别名已被其他内容使用。' });
      throw error;
    } finally { client.release(); }
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
        title: item.title, slug: item.slug, contentType: item.contentType, tags: item.tags || [],
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
        title: item.title, slug: item.slug, content_type: item.contentType, tags: item.tags || [],
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
        tags, summary, published_at AS "publishedAt"
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
      tags, summary, body, visibility, status, created_at AS "createdAt",
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
