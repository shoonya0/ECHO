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
// - "Can't judge": an _id that isn't an ObjectID, or a createdAt that is missing, isn't a
//   date, or is before 1970 (such as Go's zero time). Check these by hand.
//
// It can't find a takeover account that has since been deleted, or the takeover of an
// account that was deleted within 60 seconds of signing up.

const dbName = process.env.AUDIT_DB || "Echo";
const echo = db.getSiblingDB(dbName);
const marginMs = 60 * 1000; // ObjectID times are whole seconds; allow for that

// The time inside an _id, or null when the _id isn't an ObjectID. $convert would read a
// string as a date and a number as milliseconds, and $toDate on any other type stops the
// whole audit, so only an ObjectID reaches $toDate.
const idTime = { $cond: [{ $eq: [{ $type: "$_id" }, "objectId"] }, { $toDate: "$_id" }, null] };

// idTime and signup are null for accounts the audit can't judge. ObjectID times start in
// 1970, so an earlier createdAt can't be compared with them.
const judge = { $addFields: {
  idTime,
  signup: { $cond: [
    { $and: [{ $eq: [{ $type: "$createdAt" }, "date"] }, { $gte: ["$createdAt", new Date(0)] }] },
    "$createdAt",
    null,
  ] },
} };

const flagged = echo.users.aggregate([
  judge,
  { $match: { $expr: { $and: [
    { $ne: ["$idTime", null] },
    { $ne: ["$signup", null] },
    { $lt: ["$idTime", { $subtract: ["$signup", marginMs] }] },
  ] } } },
  { $lookup: {
    from: "messages",
    let: { uid: "$_id", created: "$signup" },
    pipeline: [
      { $match: { $expr: { $and: [
        { $eq: ["$senderId", "$$uid"] },
        { $ne: [idTime, null] },
        { $lt: [idTime, { $subtract: ["$$created", marginMs] }] },
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

const unjudged = echo.users.aggregate([
  judge,
  { $match: { $or: [{ idTime: null }, { signup: null }] } },
  { $project: { email: 1, createdAt: 1 } },
]).toArray();

print(`Database: ${dbName}, users checked: ${echo.users.countDocuments()}`);
print(`Flagged accounts: ${flagged.length}`);
flagged.forEach((u) => printjson(u));
print(`Accounts the audit can't judge: ${unjudged.length}`);
unjudged.forEach((u) => printjson(u));
