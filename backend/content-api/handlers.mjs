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
  if (path === '/resources' && req.method === 'GET') {
    const result = await pool.query(`SELECT resource_id AS "id", title, description, original_filename AS "filename", mime_type AS "mimeType", file_size AS "fileSize", created_at AS "createdAt", updated_at AS "updatedAt" FROM portal.teaching_resources WHERE owner_id = $1 AND deleted_at IS NULL ORDER BY updated_at DESC LIMIT 500`, [user.user_id]);
    return json(res, 200, { items: result.rows });
  }
  if (path === '/resources/trash' && req.method === 'GET') {
    const result = await pool.query(`SELECT resource_id AS "id", title, description, original_filename AS "filename", mime_type AS "mimeType", file_size AS "fileSize", created_at AS "createdAt", deleted_at AS "deletedAt" FROM portal.teaching_resources WHERE owner_id = $1 AND deleted_at IS NOT NULL ORDER BY deleted_at DESC LIMIT 500`, [user.user_id]);
    return json(res, 200, { items: result.rows });
  }
  if (path === '/resources' && req.method === 'POST') {
    const body = await readBody(req, 36 * 1024 * 1024);
    const title = String(body.title || '').trim(); const description = String(body.description || '').trim();
    const filename = String(body.filename || '').replace(/[\\/\x00-\x1f\x7f]/g, '').trim().slice(0, 255);
    const mime = String(body.mimeType || '').toLowerCase(); const data = String(body.data || '');
    if (!title || [...title].length > 200 || description.length > 2000 || !filename || !data || !/^[A-Za-z0-9+/]*={0,2}$/.test(data)) return json(res, 400, { error: '请填写有效的资源名称并选择文件。' });
    const bytes = Buffer.from(data, 'base64');
    if (!bytes.length || bytes.length > 25 * 1024 * 1024) return json(res, 413, { error: '单个文件大小不能超过 25 MB。' });
    const ext = filename.includes('.') ? filename.split('.').pop().toLowerCase() : '';
    const allowed = new Set(['ppt','pptx','mp3','pdf','doc','docx','txt']);
    if (!allowed.has(ext)) return json(res, 400, { error: '支持的格式：PPT、PPTX、MP3、PDF、DOC、DOCX、TXT。' });
    const signatures = {
      ppt: b => b.length >= 8 && b.subarray(0,4).toString('hex') === 'd0cf11e0',
      pptx: b => b.length >= 4 && b.subarray(0,4).toString('hex') === '504b0304',
      doc: b => b.length >= 8 && b.subarray(0,4).toString('hex') === 'd0cf11e0',
      docx: b => b.length >= 4 && b.subarray(0,4).toString('hex') === '504b0304',
      mp3: b => b.length >= 3 && (b.subarray(0,3).toString('ascii') === 'ID3' || (b[0] === 0xff && (b[1] & 0xe0) === 0xe0)),
      pdf: b => b.length >= 5 && b.subarray(0,5).toString('ascii') === '%PDF-',
      txt: b => !b.subarray(0,Math.min(b.length,8192)).includes(0),
    };
    if (!signatures[ext](bytes)) return json(res, 400, { error: '文件内容与扩展名不匹配，上传已拒绝。' });
    const fs = await import('node:fs/promises'); const pathModule = await import('node:path');
    const uploadDir = process.env.PERSONAL_PORTAL_RESOURCE_DIR || '/var/db/personal-portal/resources';
    const storedFilename = crypto.randomUUID() + '.' + ext;
    await fs.mkdir(uploadDir, { recursive: true }); await fs.writeFile(pathModule.join(uploadDir, storedFilename), bytes, { flag: 'wx', mode: 0o640 });
    try {
      const result = await pool.query(`INSERT INTO portal.teaching_resources(owner_id,title,description,original_filename,stored_filename,mime_type,file_size) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING resource_id AS "id", title, description, original_filename AS "filename", mime_type AS "mimeType", file_size AS "fileSize", created_at AS "createdAt"`, [user.user_id,title,description,filename,storedFilename,mime,bytes.length]);
      return json(res, 201, { item: result.rows[0] });
    } catch (error) { await fs.unlink(pathModule.join(uploadDir,storedFilename)).catch(() => {}); throw error; }
  }
  const resourceAction = path.match(/^\/resources\/([0-9a-f-]{36})(?:\/(restore|permanent|download))?$/i);
  if (resourceAction) {
    const [, resourceId, action] = resourceAction;
    if (req.method === 'POST' && action === 'restore') {
      const result = await pool.query(`UPDATE portal.teaching_resources SET deleted_at=NULL,updated_at=now() WHERE resource_id=$1 AND owner_id=$2 AND deleted_at IS NOT NULL RETURNING resource_id AS "id", title, description, original_filename AS "filename", mime_type AS "mimeType", file_size AS "fileSize", created_at AS "createdAt"`, [resourceId,user.user_id]);
      return result.rowCount ? json(res,200,{item:result.rows[0]}) : json(res,404,{error:'资源不存在或无权恢复。'});
    }
    if (req.method === 'DELETE' && !action) {
      const result = await pool.query(`UPDATE portal.teaching_resources SET deleted_at=now(),updated_at=now() WHERE resource_id=$1 AND owner_id=$2 AND deleted_at IS NULL RETURNING resource_id`, [resourceId,user.user_id]);
      return result.rowCount ? json(res,200,{ok:true}) : json(res,404,{error:'资源不存在或已在回收站。'});
    }
    if (req.method === 'DELETE' && action === 'permanent') {
      const result = await pool.query(`DELETE FROM portal.teaching_resources WHERE resource_id=$1 AND owner_id=$2 AND deleted_at IS NOT NULL RETURNING stored_filename`, [resourceId,user.user_id]);
      if (!result.rowCount) return json(res,404,{error:'资源不存在或不在回收站。'});
      const fs = await import('node:fs/promises'); const pathModule = await import('node:path');
      await fs.unlink(pathModule.join(process.env.PERSONAL_PORTAL_RESOURCE_DIR || '/var/db/personal-portal/resources',result.rows[0].stored_filename)).catch(() => {});
      return json(res,200,{ok:true});
    }
    if (req.method === 'GET' && action === 'download') {
      const result = await pool.query(`SELECT stored_filename,original_filename,mime_type FROM portal.teaching_resources WHERE resource_id=$1 AND owner_id=$2 AND deleted_at IS NULL`, [resourceId,user.user_id]);
      if (!result.rowCount) return json(res,404,{error:'资源不存在或无权访问。'});
      const fs = await import('node:fs'); const pathModule = await import('node:path');
      const filePath = pathModule.join(process.env.PERSONAL_PORTAL_RESOURCE_DIR || '/var/db/personal-portal/resources',result.rows[0].stored_filename);
      if (!fs.existsSync(filePath)) return json(res,404,{error:'资源文件缺失。'});
      res.writeHead(200,{'Content-Type':result.rows[0].mime_type || 'application/octet-stream','Content-Disposition':`attachment; filename*=UTF-8''${encodeURIComponent(result.rows[0].original_filename)}`,'X-Content-Type-Options':'nosniff','Cache-Control':'no-store'});
      return fs.createReadStream(filePath).pipe(res);
    }
  }
  if (path === '/content/trash' && req.method === 'GET') {
    const result = await pool.query(
      `SELECT ${contentSelect}, deleted_at AS \"deletedAt\",
         (SELECT COUNT(*)::int FROM portal.portal_content_interactions i WHERE i.content_id = portal.portal_content.content_id AND i.interaction_type = 'like') AS \"likeCount\",
         (SELECT COUNT(*)::int FROM portal.portal_content_interactions i WHERE i.content_id = portal.portal_content.content_id AND i.interaction_type = 'favorite') AS \"favoriteCount\"
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
      `SELECT ${contentSelect},
         (SELECT COUNT(*)::int FROM portal.portal_content_interactions i WHERE i.content_id = portal.portal_content.content_id AND i.interaction_type = 'like') AS "likeCount",
         (SELECT COUNT(*)::int FROM portal.portal_content_interactions i WHERE i.content_id = portal.portal_content.content_id AND i.interaction_type = 'favorite') AS "favoriteCount"
       FROM portal.portal_content
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
  if (url.pathname === '/public/content' && req.method === 'GET') {
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
  const interactionMatch = url.pathname.match(/^\/public\/content\/([a-z0-9]+(?:-[a-z0-9]+)*)\/interactions$/);
  const articleMatch = url.pathname.match(/^\/public\/content\/([a-z0-9]+(?:-[a-z0-9]+)*)$/);
  if (!articleMatch && !interactionMatch) return json(res, 404, { error: '未找到公开内容。' });
  const slug = (interactionMatch || articleMatch)[1];
  const article = await pool.query(
    `SELECT c.content_id AS "id", c.title, c.slug, c.content_type AS "contentType",
      c.tags, c.summary, c.body, c.visibility, c.status, c.created_at AS "createdAt",
      c.updated_at AS "updatedAt", c.published_at AS "publishedAt",
      u.display_name AS "authorName", u.created_at AS "authorJoinedAt"
     FROM portal.portal_content c
     JOIN portal.portal_users u ON u.user_id = c.author_id
     WHERE c.slug = $1 AND c.status = 'published' AND c.visibility = 'public' AND c.deleted_at IS NULL`,
    [slug],
  );
  if (!article.rowCount) return json(res, 404, { error: '内容不存在或尚未公开。' });
  const item = article.rows[0];
  if (interactionMatch) {
    if (req.method !== 'POST') return json(res, 405, { error: '不支持的请求方式。' });
    const user = await getSessionUser(req);
    if (!user) return json(res, 401, { error: '请先登录，再进行点赞或收藏。' });
    const body = await readBody(req, 4096);
    const type = String(body.type || '');
    if (!['like', 'favorite'].includes(type)) return json(res, 400, { error: '互动类型不正确。' });
    const existing = await pool.query(
      `DELETE FROM portal.portal_content_interactions
       WHERE content_id = $1 AND user_id = $2 AND interaction_type = $3
       RETURNING interaction_type`, [item.id, user.user_id, type]);
    const active = existing.rowCount ? false : Boolean((await pool.query(
      `INSERT INTO portal.portal_content_interactions (content_id, user_id, interaction_type)
       VALUES ($1, $2, $3) ON CONFLICT (content_id, user_id, interaction_type) DO NOTHING
       RETURNING interaction_type`, [item.id, user.user_id, type])).rowCount);
    const counts = await pool.query(
      `SELECT interaction_type, count(*)::int AS count
       FROM portal.portal_content_interactions WHERE content_id = $1 GROUP BY interaction_type`, [item.id]);
    const countMap = Object.fromEntries(counts.rows.map(row => [row.interaction_type, row.count]));
    return json(res, 200, { active, likeCount: countMap.like || 0, favoriteCount: countMap.favorite || 0 });
  }
  const counts = await pool.query(
    `SELECT interaction_type, count(*)::int AS count
     FROM portal.portal_content_interactions WHERE content_id = $1 GROUP BY interaction_type`, [item.id]);
  const countMap = Object.fromEntries(counts.rows.map(row => [row.interaction_type, row.count]));
  const user = await getSessionUser(req);
  let viewerInteractions = [];
  if (user) {
    const viewer = await pool.query(
      `SELECT interaction_type FROM portal.portal_content_interactions
       WHERE content_id = $1 AND user_id = $2`, [item.id, user.user_id]);
    viewerInteractions = viewer.rows.map(row => row.interaction_type);
  }
  item.likeCount = countMap.like || 0;
  item.favoriteCount = countMap.favorite || 0;
  item.viewerLiked = viewerInteractions.includes('like');
  item.viewerFavorited = viewerInteractions.includes('favorite');
  return json(res, 200, { item });
}

  return { handleContentRequest, handlePublicContentRequest };
}
