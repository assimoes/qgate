def greet(name, formal=False):
    if formal:
        return "Good day, " + name
    return "Hi " + name


def discount(total, member):
    if member and total > 100:
        return total * 0.9
    return total
