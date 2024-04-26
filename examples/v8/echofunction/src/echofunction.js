(subject, payload) => {
  console.log(subject);
  return {
    triggered_on: subject,
    payload: String.fromCharCode(...payload)
  }
};
