import test from 'node:test';
import assert from 'node:assert/strict';
import { portalTaxonomy, taxonomyChildren, taxonomyParentForChild } from '../src/utils/taxonomy.ts';

test('grade taxonomy contains all requested primary, middle, high school and university levels', () => {
  assert.deepEqual(taxonomyChildren('年级'), ['一年级', '二年级', '三年级', '四年级', '五年级', '六年级', '初一', '初二', '初三', '高一', '高二', '高三', '本科', '硕士']);
  assert.equal(taxonomyParentForChild('初一'), '年级');
  assert.equal(taxonomyParentForChild('硕士'), '年级');
});

test('education human-resource taxonomy contains all requested career tracks', () => {
  assert.deepEqual(taxonomyChildren('教育人力资源'), ['事业编', '国企', '省考', '国考']);
  assert.equal(taxonomyParentForChild('国考'), '教育人力资源');
});

test('each taxonomy child belongs to exactly one parent', () => {
  const children = portalTaxonomy.flatMap(category => category.children);
  assert.equal(new Set(children).size, children.length);
});
