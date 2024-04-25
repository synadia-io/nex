(subject, payload) => {
  console.log(subject);
  return {
    triggered_on: subject,
    payload: Array.prototype.slice.call(payload)
  }
};
