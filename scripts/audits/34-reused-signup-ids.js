// One-time audit for #34. Until the fix shipped on 2026-11-09, signup accepted an `_id`
// from the client, so anyone could sign up with a deleted user's ID and inherit that
// user's chats, contacts and messages. This finds accounts that may have done so.
// It only reads.
//
// Run it once against production (the URI is in the Render service's settings):
//   mongosh "<Atlas URI>" --file scripts/audits/34-reused-signup-ids.js
// AUDIT_DB picks another database than "Echo", for example a local test copy.
//
// How it works: an ObjectID records when it was made. A server-made ID matches the
// account's createdAt to within a second, and a reused ID is older. createdAt was always
// set by the server, so a client couldn't fake it.
//
// Reading the output:
// - No flagged accounts: no takeover happened.
// - messagesBeforeSignup above 0: confirmed. Messages were sent with this ID before the
//   account existed. File an issue to remove its access to the inherited chats.
// - messagesBeforeSignup of 0: suspicious, not proven. Check which chats list the ID.
// - "Can't judge": no createdAt, or an _id that isn't an ObjectID. Check these by hand.
//
// It can't find a takeover account that has since been deleted, or the takeover of an
// account that was deleted within 60 seconds of signing up.

const dbName = process.env.AUDIT_DB || "Echo";
const echo = db.getSiblingDB(dbName);
const marginMs = 60 * 1000; // ObjectID times are whole seconds; allow for that

const flagged = echo.users.aggregate([
  { $addFields: { idTime: { $convert: { input: "$_id", to: "date", onError: null, onNull: null } } } },
  { $match: { $expr: { $and: [
    { $ne: ["$idTime", null] },
    { $lt: ["$idTime", { $subtract: ["$createdAt", marginMs] }] },
  ] } } },
  { $lookup: {
    from: "messages",
    let: { uid: "$_id", created: "$createdAt" },
    pipeline: [
      { $match: { $expr: { $and: [
        { $eq: ["$senderId", "$$uid"] },
        { $lt: [{ $toDate: "$_id" }, { $subtract: ["$$created", marginMs] }] },
      ] } } },
      { $count: "n" },
    ],
    as: "before",
  } },
  { $project: {
    email: 1,
    createdAt: 1,
    idTime: 1,
    messagesBeforeSignup: { $ifNull: [{ $first: "$before.n" }, 0] },
  } },
]).toArray();

const unjudged = echo.users.find(
  { $or: [{ createdAt: { $exists: false } }, { _id: { $not: { $type: "objectId" } } }] },
  { email: 1 },
).toArray();

print(`Database: ${dbName}, users checked: ${echo.users.countDocuments()}`);
print(`Flagged accounts: ${flagged.length}`);
flagged.forEach((u) => printjson(u));
print(`Accounts the audit can't judge: ${unjudged.length}`);
unjudged.forEach((u) => printjson(u));
