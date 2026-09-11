"""Only explicitly authored local messages may cross the SDK redaction boundary."""
class UserError(ValueError):
    """A safe, actionable diagnostic that contains no credential or SDK body."""
