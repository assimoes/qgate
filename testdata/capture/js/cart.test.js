const test = require('node:test')
const assert = require('node:assert')
const { total } = require('./cart')

test('totals items', () => {
  assert.strictEqual(total([{ price: 2 }, { price: 3 }]), 5)
})
