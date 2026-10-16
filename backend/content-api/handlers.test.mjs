import test from 'node:test';
import assert from 'node:assert/strict';
import net from 'node:net';
import { readFile } from 'node:fs/promises';
import { publishEventNotification } from './handlers.mjs';
import { createContentHandlers } from './handlers.mjs';

const authorId = '11111111-1111-4111-8111-111111111111';
const contentId = '22222222-2222-4222-8222-222222222222';

test('Go event validation and Node content API use the same approved tag catalog', async () => {
  const [goSource, nodeSource] = await Promise.all([
    readFile(new URL('../api/content_validation.go', import.meta.url), 'utf8'),
    readFile(new URL('./handlers.mjs', import.meta.url), 'utf8'),
  ]);
  const goBlock = goSource.match(/var approvedContentTags = map\[string\]struct\{\}\{([\s\S]*?)\n\}/)?.[1];
  const nodeBlock = nodeSource.match(/const contentTags = new Set\(\[([\s\S]*?)\]\);/)?.[1];
  assert.ok(goBlock, 'Go approved tag map should exist');
  assert.ok(nodeBlock, 'Node approved tag set should exist');
  const goTags = [...goBlock.matchAll(/"([^"]+)": \{\}/g)].map((match) => match[1]).sort();
  const nodeTags = [...nodeBlock.matchAll(/'([^']*)'/g)].map((match) => match[1]).sort();
  assert.deepEqual(goTags, nodeTags);
});

const initialItem = () => ({
  id: contentId, title: 'Original title', slug: 'original-title', contentType: 'article', tags: ['编程', 'Python'],
  summary: 'Original summary', body: '# Original', visibility: 'private', status: 'draft',
  createdAt: '2026-01-01T00:00:00.000Z', updatedAt: '2026-01-01T00:00:00.000Z', publishedAt: null,
});

function createHarness({ initial = initialItem(), notifyEvent = async () => {} } = {}) {
  let item = structuredClone(initial);
  const versions = [];
  const events = [];
  const transactionLog = [];
  const clone = (value) => structuredClone(value);
  const client = {
    async query(sql, params = []) {
      if (/^(BEGIN|COMMIT|ROLLBACK)$/.test(sql.trim())) { transactionLog.push(sql.trim()); return { rows: [], rowCount: 0 }; }
      if (sql.includes('INSERT INTO portal.event_inbox')) { events.push({ eventId: params[0], payload: JSON.parse(params[1]) }); return { rows: [], rowCount: 1 }; }
      if (sql.includes('FOR UPDATE')) return { rows: [clone(item)], rowCount: 1 };
      if (sql.includes('COALESCE(MAX(version_number)')) return { rows: [{ next_version: versions.length + 1 }], rowCount: 1 };
      if (sql.includes('INSERT INTO portal.portal_content_versions')) {
        versions.push({ versionId: `33333333-3333-4333-8333-${String(versions.length + 1).padStart(12, '0')}`,
          versionNumber: params[2], snapshot: JSON.parse(params[3]), createdAt: `2026-01-0${versions.length + 1}T00:00:00.000Z` });
        return { rows: [], rowCount: 1 };
      }
      if (sql.includes('SELECT snapshot FROM portal.portal_content_versions')) {
        const version = versions.find((entry) => entry.versionId === params[0]);
        return { rows: version ? [{ snapshot: clone(version.snapshot) }] : [], rowCount: version ? 1 : 0 };
      }
      if (sql.includes('UPDATE portal.portal_content SET')) {
        const columns = { title: 'title', slug: 'slug', content_type: 'contentType', tags: 'tags', summary: 'summary', body: 'body', visibility: 'visibility' };
        for (const match of sql.matchAll(/(title|slug|content_type|tags|summary|body|visibility) = \$(\d+)/g)) {
          item[columns[match[1]]] = params[Number(match[2]) - 1];
        }
        item.updatedAt = '2026-10-15T00:00:00.000Z';
        return { rows: [clone(item)], rowCount: 1 };
      }
      throw new Error(`Unexpected client query: ${sql}`);
    },
    release() {},
  };
  const pool = {
    async connect() { return client; },
    async query(sql, params = []) {
      if (sql.includes('SELECT content_id FROM portal.portal_content WHERE content_id = $1 AND author_id = $2')) return { rows: [{ content_id: contentId }], rowCount: 1 };
      if (sql.includes('SELECT version_id AS')) return { rows: versions.map((v) => ({ versionId: v.versionId, versionNumber: v.versionNumber, title: v.snapshot.title, slug: v.snapshot.slug, status: v.snapshot.status, createdAt: v.createdAt })).reverse(), rowCount: versions.length };
      if (sql.includes('deleted_at IS NOT NULL')) return { rows: [], rowCount: 0 };
      throw new Error(`Unexpected pool query: ${sql}`);
    },
  };
  const handlers = createContentHandlers({
    pool,
    getSessionUser: async () => ({ user_id: authorId }),
    json: (_res, status, payload) => ({ status, payload }),
    readBody: async (req) => req.body || {},
    notifyEvent,
  });
  async function call(method, path, body) {
    return handlers.handleContentRequest({ method, body }, {}, new URL(`http://local${path}`));
  }
  return { call, versions, events, transactionLog, current: () => clone(item) };
}

test('changed edits snapshot the prior content and version listing returns owner history', async () => {
  const h = createHarness();
  const edited = await h.call('PATCH', `/content/${contentId}`, { title: 'Updated title', body: '# Updated' });
  assert.equal(edited.status, 200);
  assert.equal(edited.payload.versionCreated, true);
  assert.equal(h.versions.length, 1);
  assert.equal(h.versions[0].snapshot.title, 'Original title');
  const listed = await h.call('GET', `/content/${contentId}/versions`);
  assert.equal(listed.status, 200);
  assert.equal(listed.payload.items[0].title, 'Original title');
});

test('restoring a historical version snapshots the current state first', async () => {
  const h = createHarness();
  await h.call('PATCH', `/content/${contentId}`, { title: 'Updated title' });
  const priorVersionId = h.versions[0].versionId;
  const restored = await h.call('POST', `/content/${contentId}/versions/${priorVersionId}/restore`);
  assert.equal(restored.status, 200);
  assert.equal(restored.payload.item.title, 'Original title');
  assert.equal(h.versions.length, 2);
  assert.equal(h.versions[1].snapshot.title, 'Updated title');
});

test('saving unchanged fields does not create a duplicate history entry', async () => {
  const h = createHarness();
  const result = await h.call('PATCH', `/content/${contentId}`, { title: 'Original title' });
  assert.equal(result.status, 200);
  assert.equal(result.payload.versionCreated, false);
  assert.equal(h.versions.length, 0);
});

test('new content requires at least one approved Knowledge Plaza tag', async () => {
  const h = createHarness();
  const missing = await h.call('POST', '/content', { title: 'Tagged content', slug: 'tagged-content', contentType: 'article', body: '# Content' });
  assert.equal(missing.status, 400);
  assert.match(missing.payload.error, /至少选择一个知识广场标签/);
  const invalid = await h.call('PATCH', `/content/${contentId}`, { tags: ['not-a-real-tag'] });
  assert.equal(invalid.status, 400);
});


test('saving an unchanged tag array does not create a duplicate history entry', async () => {
  const h = createHarness();
  const result = await h.call('PATCH', `/content/${contentId}`, { tags: ['编程', 'Python'] });
  assert.equal(result.status, 200);
  assert.equal(result.payload.versionCreated, false);
  assert.equal(h.versions.length, 0);
});


test('event notification publisher sends PUBLISH and validates the acknowledgement', async () => {
  const socketPath = `/tmp/portal-publish-test-${process.pid}-${Date.now()}.sock`;
  const server = net.createServer((socket) => {
    let request = '';
    socket.on('data', (chunk) => {
      request += chunk.toString('ascii');
      if (!request.includes('\n')) return;
      const fields = request.trim().split(/\s+/);
      assert.equal(fields[0], 'v1');
      assert.equal(fields[2], 'PUBLISH');
      assert.equal(fields[3], '018f2f5e-7a2b-7c4d-8e9f-0123456789ab');
      assert.equal(fields[4], 'portal.content.validate');
      socket.end(`v1 ${fields[1]} OK PUBLISHED\n`);
    });
  });
  await new Promise((resolve, reject) => server.listen(socketPath, resolve).once('error', reject));
  try {
    await publishEventNotification('018f2f5e-7a2b-7c4d-8e9f-0123456789ab', 'portal.content.validate', socketPath);
  } finally {
    await new Promise((resolve) => server.close(resolve));
  }
});


test('published historical restore commits its event before notifying C; notification failure does not undo the restore', async () => {
  const order = [];
  const h = createHarness({
    initial: { ...initialItem(), status: 'published', visibility: 'public' },
    notifyEvent: async (eventId, eventType) => {
      order.push('notify');
      assert.equal(eventId, h.events[0].eventId);
      assert.equal(eventType, 'portal.content.validate');
      assert.equal(h.transactionLog.at(-1), 'COMMIT', 'notification must only happen after COMMIT');
      throw new Error('simulated C IPC outage');
    },
  });
  await h.call('PATCH', `/content/${contentId}`, { title: 'Updated published title' });
  const versionId = h.versions[0].versionId;
  const restored = await h.call('POST', `/content/${contentId}/versions/${versionId}/restore`);
  assert.equal(restored.status, 200);
  assert.equal(restored.payload.item.title, 'Original title');
  assert.equal(h.events.length, 1);
  assert.ok(h.transactionLog.lastIndexOf('COMMIT') >= 0);
  assert.equal(order[0], 'notify');
  assert.equal(restored.payload.eventId, h.events[0].eventId);
  await new Promise((resolve) => setImmediate(resolve));
});
