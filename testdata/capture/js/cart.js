function total(items, coupon) {
  const unused = 42
  let sum = 0
  for (const i of items) sum += i.price
  if (coupon) {
    sum -= coupon.amount
  }
  console.log('total', sum)
  return sum
}

module.exports = { total }
